package ads

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/media"
	"athlix/backend/internal/platform/userdir"
)

// ist is India Standard Time; stat days follow the IST calendar.
var ist = time.FixedZone("IST", 5*3600+1800)

const (
	maxStatsDays     = 366
	defaultStatsDays = 30
	endBatch         = 500 // docs/architecture.md: jobs touch at most 500 rows per run
)

// Service holds the ads business rules.
type Service struct {
	d    *app.Deps
	repo *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, repo: &repo{db: d.DB}} }

func (s *Service) now() time.Time { return s.d.Now().UTC() }

// showsTo reports whether ads may be shown to this user at all (ads.enabled, minor rule).
func (s *Service) showsTo(ctx context.Context, u userdir.Basic) bool {
	if !s.d.Settings.Bool(ctx, "ads.enabled", false) {
		return false
	}
	if u.IsMinor && !s.d.Settings.Bool(ctx, "ads.show_to_minors", false) {
		return false
	}
	return true
}

// Serve returns the ads for one placement (contextual targeting only).
func (s *Service) Serve(ctx context.Context, u userdir.Basic, placement string, sportID int64, state string) ([]Ad, error) {
	if !slices.Contains(Placements, placement) {
		return nil, httpx.Validation(map[string]string{"placement": "must be one of " + strings.Join(Placements, ", ")})
	}
	if utf8.RuneCountInString(state) > 64 {
		return nil, httpx.Validation(map[string]string{"state": "is too long"})
	}
	out := []Ad{}
	if !s.showsTo(ctx, u) {
		return out, nil
	}
	rows, err := s.repo.serving(ctx, s.now(), placement, sportID, strings.TrimSpace(state), max(1, min(s.d.Settings.Int(ctx, "ads.max_per_request", 3), maxServedCeiling)))
	if err != nil {
		return nil, httpx.Internal().Wrap(err)
	}
	urls := s.d.Media.ViewURLs(ctx, imageIDs(rows))
	for _, c := range rows {
		a := Ad{ID: c.ID, AdvertiserName: c.AdvertiserName, Title: c.Title, Body: c.Body, TargetURL: c.TargetURL, Placement: c.Placement, Label: Label}
		if c.ImageUploadID != nil {
			if v, ok := urls[*c.ImageUploadID]; ok {
				a.ImageURL = &v
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// Track counts one impression or click on a servable ad.
func (s *Service) Track(ctx context.Context, u userdir.Basic, id int64, click bool) error {
	if !s.showsTo(ctx, u) {
		return httpx.NotFound("Ad")
	}
	now := s.now()
	ok, err := s.repo.isServing(ctx, now, id)
	if err != nil {
		return httpx.Internal().Wrap(err)
	}
	if !ok {
		return httpx.NotFound("Ad")
	}
	if err := s.repo.bump(ctx, id, now.In(ist).Format(time.DateOnly), click); err != nil {
		return httpx.Internal().Wrap(err)
	}
	return nil
}

// EndCampaigns is the ads.end_campaigns job: active / paused campaigns past ends_at → ended.
// Serving already stops at ends_at; this keeps the admin list status true. Idempotent.
func (s *Service) EndCampaigns(ctx context.Context) error {
	n, err := s.repo.endPast(ctx, s.now(), endBatch)
	if err != nil {
		return err
	}
	logger.From(ctx).Info("ads.end_campaigns done", slog.Int64("ended", n))
	return nil
}

// --- admin ---

// List pages campaigns for the admin panel.
func (s *Service) List(ctx context.Context, status string, p httpx.PageParams) ([]Campaign, string, error) {
	if status != "" && !slices.Contains(Statuses, status) {
		return nil, "", httpx.Validation(map[string]string{"status": "must be one of " + strings.Join(Statuses, ", ")})
	}
	rows, err := s.repo.list(ctx, status, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", httpx.Internal().Wrap(err)
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	s.attachImages(ctx, rows)
	if rows == nil {
		rows = []Campaign{}
	}
	return rows, next, nil
}

// Create adds a campaign.
func (s *Service) Create(c *gin.Context, in CampaignInput) (Campaign, error) {
	ctx := c.Request.Context()
	fields := map[string]string{}
	for k, missing := range map[string]bool{
		"advertiserName": in.AdvertiserName == nil, "title": in.Title == nil, "targetUrl": in.TargetURL == nil,
		"placement": in.Placement == nil, "startsAt": in.StartsAt == nil, "endsAt": in.EndsAt == nil,
	} {
		if missing {
			fields[k] = "is required"
		}
	}
	if len(fields) > 0 {
		return Campaign{}, httpx.Validation(fields)
	}
	camp := Campaign{Status: "draft"}
	apply(&camp, in)
	var id int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		claim := int64(0)
		if camp.ImageUploadID != nil {
			claim = *camp.ImageUploadID
		}
		if err := s.validate(ctx, tx, camp, camp.SportID != nil, claim); err != nil {
			return err
		}
		var err error
		if id, err = s.repo.insert(ctx, tx, camp, authn.AdminID(c)); err != nil {
			return err
		}
		return audit.Admin(c, tx, "ads.create", "ad_campaign", strconv.FormatInt(id, 10), map[string]any{"status": camp.Status, "placement": camp.Placement})
	})
	if err != nil {
		return Campaign{}, asHTTP(err)
	}
	logger.From(ctx).Info("ad campaign created", slog.Int64("adId", id), slog.Int64("adminId", authn.AdminID(c)))
	return s.Get(ctx, id)
}

// Update changes a campaign (partial).
func (s *Service) Update(c *gin.Context, id int64, in CampaignInput) (Campaign, error) {
	ctx := c.Request.Context()
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		camp, err := s.repo.get(ctx, tx, id, true)
		if db.IsNoRows(err) {
			return httpx.NotFound("Ad")
		}
		if err != nil {
			return err
		}
		oldImage := camp.ImageUploadID
		apply(&camp, in)
		claimFrom := int64(0)
		if camp.ImageUploadID != nil && (oldImage == nil || *oldImage != *camp.ImageUploadID) {
			claimFrom = *camp.ImageUploadID
		}
		if err := s.validate(ctx, tx, camp, in.SportID.Set && camp.SportID != nil, claimFrom); err != nil {
			return err
		}
		if err := s.repo.update(ctx, tx, camp); err != nil {
			return err
		}
		return audit.Admin(c, tx, "ads.update", "ad_campaign", strconv.FormatInt(id, 10), map[string]any{"status": camp.Status})
	})
	if err != nil {
		return Campaign{}, asHTTP(err)
	}
	return s.Get(ctx, id)
}

// Get loads one campaign with its image URL.
func (s *Service) Get(ctx context.Context, id int64) (Campaign, error) {
	camp, err := s.repo.get(ctx, s.d.DB, id, false)
	if db.IsNoRows(err) {
		return Campaign{}, httpx.NotFound("Ad")
	}
	if err != nil {
		return Campaign{}, httpx.Internal().Wrap(err)
	}
	rows := []Campaign{camp}
	s.attachImages(ctx, rows)
	return rows[0], nil
}

// Stats returns daily counters for [from, to] (IST dates), zero-filled.
func (s *Service) Stats(ctx context.Context, id int64, fromS, toS string) ([]DayStat, error) {
	today := s.now().In(ist)
	to := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	from := to.AddDate(0, 0, -(defaultStatsDays - 1))
	fields := map[string]string{}
	if fromS != "" {
		t, err := time.Parse(time.DateOnly, fromS)
		if err != nil {
			fields["from"] = "must be YYYY-MM-DD"
		}
		from = t
	}
	if toS != "" {
		t, err := time.Parse(time.DateOnly, toS)
		if err != nil {
			fields["to"] = "must be YYYY-MM-DD"
		}
		to = t
	}
	if len(fields) == 0 {
		switch {
		case to.Before(from):
			fields["to"] = "must not be before from"
		case int(to.Sub(from).Hours()/24)+1 > maxStatsDays:
			fields["to"] = "range must be at most 366 days"
		}
	}
	if len(fields) > 0 {
		return nil, httpx.Validation(fields)
	}
	if _, err := s.repo.get(ctx, s.d.DB, id, false); err != nil {
		if db.IsNoRows(err) {
			return nil, httpx.NotFound("Ad")
		}
		return nil, httpx.Internal().Wrap(err)
	}
	got, err := s.repo.stats(ctx, id, from.Format(time.DateOnly), to.Format(time.DateOnly))
	if err != nil {
		return nil, httpx.Internal().Wrap(err)
	}
	out := []DayStat{}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		k := d.Format(time.DateOnly)
		if v, ok := got[k]; ok {
			out = append(out, v)
		} else {
			out = append(out, DayStat{Date: k})
		}
	}
	return out, nil
}

// apply copies the fields present in in onto c.
func apply(c *Campaign, in CampaignInput) {
	if in.AdvertiserName != nil {
		c.AdvertiserName = strings.TrimSpace(*in.AdvertiserName)
	}
	if in.Title != nil {
		c.Title = strings.TrimSpace(*in.Title)
	}
	if in.Body.Set {
		c.Body = nil
		if v := strings.TrimSpace(in.Body.V); in.Body.Valid && v != "" {
			c.Body = &v
		}
	}
	if in.ImageUploadID.Set {
		c.ImageUploadID = nil
		if v := in.ImageUploadID.V; in.ImageUploadID.Valid {
			c.ImageUploadID = &v
		}
	}
	if in.TargetURL != nil {
		c.TargetURL = strings.TrimSpace(*in.TargetURL)
	}
	if in.Placement != nil {
		c.Placement = *in.Placement
	}
	if in.SportID.Set {
		c.SportID = nil
		if v := in.SportID.V; in.SportID.Valid {
			c.SportID = &v
		}
	}
	if in.State.Set {
		c.State = nil
		if v := strings.TrimSpace(in.State.V); in.State.Valid && v != "" {
			c.State = &v
		}
	}
	if in.Priority != nil {
		c.Priority = *in.Priority
	}
	if in.StartsAt != nil {
		c.StartsAt = in.StartsAt.UTC()
	}
	if in.EndsAt != nil {
		c.EndsAt = in.EndsAt.UTC()
	}
	if in.Status != nil {
		c.Status = *in.Status
	}
}

// validate checks the merged campaign. checkSport looks the sport up (only when it changed);
// claimImage, when non-zero, is a newly named image upload to claim.
func (s *Service) validate(ctx context.Context, tx *sql.Tx, c Campaign, checkSport bool, claimImage int64) error {
	f := map[string]string{}
	textLen := func(key, v string, max int, required bool) {
		n := utf8.RuneCountInString(v)
		switch {
		case required && n == 0:
			f[key] = "is required"
		case n > max:
			f[key] = "must be at most " + strconv.Itoa(max) + " characters"
		}
	}
	textLen("advertiserName", c.AdvertiserName, 120, true)
	textLen("title", c.Title, 120, true)
	if c.Body != nil {
		textLen("body", *c.Body, 300, false)
	}
	if c.State != nil {
		textLen("state", *c.State, 64, false)
	}
	if err := checkTargetURL(c.TargetURL); err != "" {
		f["targetUrl"] = err
	}
	if !slices.Contains(Placements, c.Placement) {
		f["placement"] = "must be one of " + strings.Join(Placements, ", ")
	}
	if !slices.Contains(Statuses, c.Status) {
		f["status"] = "must be one of " + strings.Join(Statuses, ", ")
	}
	if c.Priority < -1000 || c.Priority > 1000 {
		f["priority"] = "must be between -1000 and 1000"
	}
	if !c.StartsAt.Before(c.EndsAt) {
		f["endsAt"] = "must be after startsAt"
	}
	if c.SportID != nil && *c.SportID <= 0 {
		f["sportId"] = "is invalid"
	}
	if c.ImageUploadID != nil && *c.ImageUploadID <= 0 {
		f["imageUploadId"] = "is invalid"
	}
	if len(f) > 0 {
		return httpx.Validation(f)
	}
	if checkSport {
		ok, err := s.repo.sportExists(ctx, tx, *c.SportID)
		if err != nil {
			return err
		}
		if !ok {
			return httpx.Validation(map[string]string{"sportId": "is not a known sport"})
		}
	}
	if claimImage > 0 {
		// Admin uploads are owned by user id 0 (docs/api.md, admin media uploads).
		if err := s.d.Media.Claim(ctx, tx, 0, claimImage, media.PurposePostImage); err != nil {
			if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrNotUploaded) || errors.Is(err, media.ErrMismatch) {
				return httpx.Validation(map[string]string{"imageUploadId": "is not a usable ad image upload"})
			}
			return err
		}
	}
	return nil
}

func checkTargetURL(v string) string {
	if v == "" {
		return "is required"
	}
	if len(v) > 500 {
		return "must be at most 500 characters"
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "must be an absolute https:// URL"
	}
	return ""
}

func (s *Service) attachImages(ctx context.Context, rows []Campaign) {
	urls := s.d.Media.ViewURLs(ctx, imageIDs(rows))
	for i := range rows {
		if id := rows[i].ImageUploadID; id != nil {
			if v, ok := urls[*id]; ok {
				rows[i].ImageURL = &v
			}
		}
	}
}

func imageIDs(rows []Campaign) []int64 {
	var ids []int64
	for _, c := range rows {
		if c.ImageUploadID != nil {
			ids = append(ids, *c.ImageUploadID)
		}
	}
	return ids
}

func asHTTP(err error) error {
	if _, ok := httpx.As(err); ok {
		return err
	}
	return httpx.Internal().Wrap(err)
}

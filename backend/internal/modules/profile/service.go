package profile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/sport"
	"athlix/backend/internal/modules/user"
	"athlix/backend/internal/modules/verification"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/entitlements"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	pmedia "athlix/backend/internal/platform/media"
	"athlix/backend/internal/platform/userdir"
)

// Service implements profiles, stats, achievements and profile views.
type Service struct {
	d    *app.Deps
	repo *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, repo: &repo{db: d.DB}} }

// ist is used for profile-view days and "today" checks.
var ist = time.FixedZone("IST", 5*3600+1800)

const (
	dateLayout    = "2006-01-02"
	badgeCoachPro = "coach_pro"
	maxStatText   = 100
)

var seasonRe = regexp.MustCompile(`^\d{4}(-\d{2})?$`)

// ---- small validation helpers ----

// text trims s and checks its length in characters; control characters are refused.
func text(fields map[string]string, key, s string, min, max int) string {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	switch {
	case n == 0 && min > 0:
		fields[key] = "is required"
	case n < min:
		fields[key] = "is too short"
	case n > max:
		fields[key] = "is too long"
	case strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) >= 0:
		fields[key] = "contains invalid characters"
	}
	return s
}

func optText(fields map[string]string, key string, s *string, max int) string {
	if s == nil {
		return ""
	}
	return text(fields, key, *s, 0, max)
}

func (s *Service) today() time.Time {
	n := s.d.Now().In(ist)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// minorDefault is minor.profile_default_visibility (falls back to followers).
func (s *Service) minorDefault(ctx context.Context) string {
	v := s.d.Settings.String(ctx, "minor.profile_default_visibility", "followers")
	if !slices.Contains(Visibilities, v) {
		return "followers"
	}
	return v
}

// claimErr maps platform/media claim errors to a 422 on field.
func claimErr(err error, field string) error {
	switch {
	case errors.Is(err, pmedia.ErrNotFound):
		return httpx.Validation(map[string]string{field: "is not one of your uploads of the right kind"})
	case errors.Is(err, pmedia.ErrNotUploaded):
		return httpx.Validation(map[string]string{field: "has not been uploaded yet"})
	case errors.Is(err, pmedia.ErrMismatch):
		return httpx.Validation(map[string]string{field: "does not match the declared file; upload it again"})
	}
	return err
}

// checkSport validates a sport and optional position against the active catalogue.
func (s *Service) checkSport(ctx context.Context, q db.Querier, fields map[string]string, sportKey, posKey string, sportID int64, posID *int64) error {
	ok, err := s.repo.activeSport(ctx, q, sportID)
	if err != nil {
		return err
	}
	if !ok {
		fields[sportKey] = "is not an active sport"
		return nil
	}
	if posID != nil && *posID != 0 {
		ok, err := s.repo.activePosition(ctx, q, sportID, *posID)
		if err != nil {
			return err
		}
		if !ok {
			fields[posKey] = "is not a position of this sport"
		}
	}
	return nil
}

// ---- onboarding ----

// Setup is POST /onboarding/profile: creates the profile and finishes onboarding.
func (s *Service) Setup(ctx context.Context, me userdir.Basic, in setupBody) (user.Me, error) {
	fields := map[string]string{}
	name := text(fields, "displayName", in.DisplayName, 2, 80)
	if in.SportID <= 0 {
		fields["sportId"] = "is required"
	}
	np := newProfile{
		UserID:      me.ID,
		SportID:     in.SportID,
		PositionID:  in.PositionID,
		City:        optText(fields, "city", in.City, 64),
		District:    optText(fields, "district", in.District, 64),
		State:       optText(fields, "state", in.State, 64),
		Bio:         optText(fields, "bio", in.Bio, 500),
		AcademyClub: optText(fields, "academyClub", in.AcademyClub, 120),
		Visibility:  "public",
	}
	if in.PhotoUploadID != nil && *in.PhotoUploadID <= 0 {
		fields["photoUploadId"] = "is invalid"
	}
	if len(fields) > 0 {
		return user.Me{}, httpx.Validation(fields)
	}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		step, err := s.repo.lockStep(ctx, tx, me.ID)
		if db.IsNoRows(err) {
			return httpx.NotFound("User")
		}
		if err != nil {
			return err
		}
		if step == user.StepDone {
			return httpx.InvalidState("Your profile is already set up.")
		}
		if step != user.StepProfile {
			return httpx.InvalidState("Finish the earlier onboarding steps first.")
		}
		if err := s.checkSport(ctx, tx, fields, "sportId", "positionId", in.SportID, in.PositionID); err != nil {
			return err
		}
		if len(fields) > 0 {
			return httpx.Validation(fields)
		}
		if y, ok, err := verification.BirthYear(ctx, s.d, tx, me.ID); err != nil {
			return err
		} else if ok {
			np.BirthYear = &y
		}
		if me.IsMinor {
			np.Visibility = s.minorDefault(ctx)
		}
		np.StatsVisib = np.Visibility
		if in.PhotoUploadID != nil {
			if err := s.d.Media.Claim(ctx, tx, me.ID, *in.PhotoUploadID, pmedia.PurposeProfilePhoto); err != nil {
				return claimErr(err, "photoUploadId")
			}
			np.PhotoUploadID = in.PhotoUploadID
		}
		if err := s.repo.insertProfile(ctx, tx, np); err != nil {
			if db.IsDuplicate(err) {
				return httpx.InvalidState("Your profile is already set up.")
			}
			return err
		}
		if err := s.repo.insertSport(ctx, tx, me.ID, in.SportID, in.PositionID, true); err != nil {
			return err
		}
		if err := user.SetDisplayName(ctx, tx, me.ID, name); err != nil {
			return err
		}
		_, err = user.Advance(ctx, s.d, tx, me.ID)
		return err
	})
	if err != nil {
		return user.Me{}, err
	}
	s.d.Users.Invalidate(me.ID)
	return user.LoadMe(ctx, s.d, me.ID)
}

// ---- viewing ----

// subject is a profile owner as seen by a viewer.
type subject struct {
	target     userdir.Basic
	row        row
	own        bool
	following  bool // viewer follows target
	followedBy bool // target follows viewer
}

// visible applies a visibility value to the viewer.
func (sj subject) visible(v string) bool {
	switch {
	case sj.own:
		return true
	case v == "public":
		return true
	case v == "followers":
		return sj.following
	}
	return false
}

// load resolves targetID for viewer: deleted / suspended / unfinished users, users without a
// profile and blocks either way are all 404, with the same response.
func (s *Service) load(ctx context.Context, viewer userdir.Basic, targetID int64) (subject, error) {
	sj := subject{own: viewer.ID == targetID}
	t, err := s.d.Users.Get(ctx, targetID)
	if db.IsNoRows(err) {
		return sj, httpx.NotFound("Profile")
	}
	if err != nil {
		return sj, err
	}
	if t.Status == "deleted" || (!sj.own && (t.Status != "active" || t.OnboardingStep != user.StepDone)) {
		return sj, httpx.NotFound("Profile")
	}
	sj.target = t
	if !sj.own {
		blocked, err := s.d.Users.BlockedEitherWay(ctx, viewer.ID, targetID)
		if err != nil {
			return sj, err
		}
		if blocked {
			return sj, httpx.NotFound("Profile")
		}
	}
	sj.row, err = s.repo.load(ctx, s.d.DB, targetID, false)
	if db.IsNoRows(err) {
		return sj, httpx.NotFound("Profile")
	}
	if err != nil {
		return sj, err
	}
	if !sj.own {
		if sj.following, err = s.d.Users.Follows(ctx, viewer.ID, targetID); err != nil {
			return sj, err
		}
		if sj.followedBy, err = s.d.Users.Follows(ctx, targetID, viewer.ID); err != nil {
			return sj, err
		}
	}
	return sj, nil
}

// View is GET /profiles/:userId.
func (s *Service) View(ctx context.Context, viewer userdir.Basic, targetID int64) (Profile, error) {
	sj, err := s.load(ctx, viewer, targetID)
	if err != nil {
		return Profile{}, err
	}
	card, ok, err := s.d.Users.Summary(ctx, targetID)
	if err != nil {
		return Profile{}, err
	}
	if !ok {
		return Profile{}, httpx.NotFound("Profile")
	}
	if sj.target.IsMinor && !sj.own && !sj.following {
		card.City = nil
	}
	p := Profile{
		User: card, IsOwn: sj.own, Sports: []ProfileSport{}, Badges: []string{},
		FollowersCount: sj.row.FollowersCount, FollowingCount: sj.row.FollowingCount,
		IsFollowing: sj.following, IsFollowedBy: sj.followedBy,
	}
	if !sj.own && !viewer.IsMinor {
		if err := s.repo.recordView(ctx, targetID, viewer.ID, s.today().Format(dateLayout)); err != nil {
			logger.From(ctx).Warn("record profile view", slog.Int64("viewedId", targetID), slog.Int64("viewerId", viewer.ID), slog.String("err", err.Error()))
		}
	}
	if !sj.visible(sj.row.Visibility) {
		p.Restricted = true
		return p, nil
	}
	r := sj.row
	p.Bio, p.District, p.AcademyClub = strPtr(r.Bio), strPtr(r.District), strPtr(r.AcademyClub)
	p.Visibility, p.StatsVisibility = &r.Visibility, &r.StatsVisibility
	p.CoachingSinceYear = intPtr(r.CoachingSinceYear)
	if sj.own {
		p.BirthYear = intPtr(r.BirthYear)
	}
	if p.Sports, err = s.repo.sports(ctx, s.d.DB, targetID); err != nil {
		return Profile{}, err
	}
	if has, err := s.d.Entitlements.Has(ctx, targetID, entitlements.VerifiedCoachBadge); err != nil {
		return Profile{}, err
	} else if has {
		p.Badges = append(p.Badges, badgeCoachPro)
	}
	if sj.target.HasRole("coach") {
		offered, err := s.repo.consultationOffered(ctx, targetID)
		if err != nil {
			return Profile{}, err
		}
		p.Consultation = &Consultation{Offered: offered}
	}
	if sj.target.HasRole("organiser") {
		o, err := s.repo.organiser(ctx, targetID)
		if err != nil {
			return Profile{}, err
		}
		p.Organiser = &o
	}
	if !sj.own {
		if p.CanMessage, err = s.canMessage(ctx, viewer, sj); err != nil {
			return Profile{}, err
		}
	}
	return p, nil
}

// canMessage applies the DM rules of docs/api.md "Chat" (rule 1, blocks, is already checked by
// load).
func (s *Service) canMessage(ctx context.Context, viewer userdir.Basic, sj subject) (bool, error) {
	if viewer.Status != "active" || sj.target.Status != "active" || sj.target.OnboardingStep != user.StepDone {
		return false, nil
	}
	st, err := user.LoadSettings(ctx, s.d.DB, sj.target.ID)
	if err != nil {
		return false, err
	}
	allow := st.Privacy.AllowDmsFrom
	if allow == "everyone" && s.d.Settings.Bool(ctx, "chat.dm_requires_follow", false) {
		allow = "followers"
	}
	mutual := sj.following && sj.followedBy
	var rule2 bool
	switch allow {
	case "everyone":
		rule2 = true
	case "followers":
		rule2 = sj.followedBy
	case "mutual":
		rule2 = mutual
	}
	if viewer.IsMinor || sj.target.IsMinor {
		switch s.d.Settings.String(ctx, "minor.dm_policy", "mutual_follow") {
		case "anyone":
			return rule2, nil
		case "none":
			return false, nil
		default: // mutual_follow, and the safe choice for an unknown value
			return rule2 && mutual, nil
		}
	}
	if rule2 {
		return true, nil
	}
	return s.repo.acceptedConsultation(ctx, viewer.ID, sj.target.ID)
}

// ---- update ----

// Update is PUT /me/profile.
func (s *Service) Update(ctx context.Context, me userdir.Basic, in updateBody) (Profile, error) {
	fields := map[string]string{}
	var cols []string
	var args []any
	set := func(col string, v any) { cols = append(cols, col); args = append(args, v) }

	var name string
	if in.DisplayName != nil {
		name = text(fields, "displayName", *in.DisplayName, 2, 80)
	}
	if in.Bio != nil {
		set("bio", db.NullString(optText(fields, "bio", in.Bio, 500)))
	}
	if in.City != nil {
		set("city", db.NullString(optText(fields, "city", in.City, 64)))
	}
	if in.District != nil {
		set("district", db.NullString(optText(fields, "district", in.District, 64)))
	}
	if in.State != nil {
		set("state", db.NullString(optText(fields, "state", in.State, 64)))
	}
	if in.AcademyClub != nil {
		set("academyClub", db.NullString(optText(fields, "academyClub", in.AcademyClub, 120)))
	}
	minorDefault := ""
	if me.IsMinor {
		minorDefault = s.minorDefault(ctx)
	}
	checkVis := func(key string, v *string) {
		if v == nil {
			return
		}
		if !slices.Contains(Visibilities, *v) {
			fields[key] = "must be one of " + strings.Join(Visibilities, ", ")
			return
		}
		if me.IsMinor && *v == "public" && minorDefault != "public" {
			fields[key] = "cannot be public for under-18 accounts"
			return
		}
		set(key, *v)
	}
	checkVis("visibility", in.Visibility)
	checkVis("statsVisibility", in.StatsVisibility)
	if in.CoachingSinceYear != nil {
		y := *in.CoachingSinceYear
		switch {
		case !me.HasRole("coach"):
			fields["coachingSinceYear"] = "is only for coaches"
		case y < 1900 || y > s.today().Year():
			fields["coachingSinceYear"] = "is not a valid year"
		default:
			set("coachingSinceYear", y)
		}
	}
	var primary *sportInput
	if in.Sports != nil {
		list := *in.Sports
		seen := map[int64]bool{}
		for i, sp := range list {
			if sp.SportID <= 0 || seen[sp.SportID] {
				fields["sports"] = "must list each sport once"
				break
			}
			seen[sp.SportID] = true
			if sp.IsPrimary {
				if primary != nil {
					fields["sports"] = "can have only one primary sport"
					break
				}
				primary = &list[i]
			}
		}
		if len(list) == 0 {
			fields["sports"] = "must have at least one sport"
		} else if primary == nil {
			list[0].IsPrimary = true
			primary = &list[0]
		}
	}
	if in.PhotoUploadID != nil && *in.PhotoUploadID < 0 {
		fields["photoUploadId"] = "is invalid"
	}
	if len(fields) > 0 {
		return Profile{}, httpx.Validation(fields)
	}

	var oldPhoto int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.load(ctx, tx, me.ID, true)
		if db.IsNoRows(err) {
			return httpx.NotFound("Profile")
		}
		if err != nil {
			return err
		}
		if in.Sports != nil {
			for _, sp := range *in.Sports {
				if err := s.checkSport(ctx, tx, fields, "sports", "sports", sp.SportID, sp.PositionID); err != nil {
					return err
				}
			}
			if len(fields) > 0 {
				return httpx.Validation(fields)
			}
			set("sportId", primary.SportID)
			set("positionId", nullPtr(primary.PositionID))
		}
		if in.PhotoUploadID != nil && *in.PhotoUploadID != cur.PhotoUploadID.Int64 {
			if *in.PhotoUploadID > 0 {
				if err := s.d.Media.Claim(ctx, tx, me.ID, *in.PhotoUploadID, pmedia.PurposeProfilePhoto); err != nil {
					return claimErr(err, "photoUploadId")
				}
			}
			set("photoUploadId", nullPtr(in.PhotoUploadID))
			oldPhoto = cur.PhotoUploadID.Int64
		}
		if err := s.repo.update(ctx, tx, me.ID, cols, args); err != nil {
			return err
		}
		if in.Sports != nil {
			if err := s.repo.deleteSports(ctx, tx, me.ID); err != nil {
				return err
			}
			for _, sp := range *in.Sports {
				if err := s.repo.insertSport(ctx, tx, me.ID, sp.SportID, sp.PositionID, sp.IsPrimary); err != nil {
					return err
				}
			}
		}
		if in.DisplayName != nil {
			return user.SetDisplayName(ctx, tx, me.ID, name)
		}
		return nil
	})
	if err != nil {
		return Profile{}, err
	}
	if oldPhoto != 0 {
		s.purge(ctx, me.ID, oldPhoto)
	}
	s.d.Users.Invalidate(me.ID)
	return s.View(ctx, me, me.ID)
}

// purge deletes a replaced upload; failures are logged (the row is already detached).
func (s *Service) purge(ctx context.Context, userID, uploadID int64) {
	if err := s.d.Media.Purge(ctx, uploadID); err != nil {
		logger.From(ctx).Warn("purge replaced upload", slog.Int64("userId", userID), slog.Int64("uploadId", uploadID), slog.String("err", err.Error()))
	}
}

// ---- profile viewers ----

// Viewers is GET /me/profile-viewers (benefit see_profile_viewers).
func (s *Service) Viewers(ctx context.Context, me userdir.Basic, page httpx.PageParams) ([]Viewer, string, error) {
	has, err := s.d.Entitlements.Has(ctx, me.ID, entitlements.SeeProfileViewers)
	if err != nil {
		return nil, "", err
	}
	if !has {
		e := httpx.Coded(http.StatusForbidden, httpx.CodePlanRequired, "Upgrade to Premium to see who viewed your profile.")
		e.Fields = map[string]string{"benefit": entitlements.SeeProfileViewers}
		return nil, "", e
	}
	if page.Cursor.Time != "" {
		if _, err := time.Parse(dateLayout, page.Cursor.Time); err != nil {
			return nil, "", httpx.Validation(map[string]string{"cursor": "is invalid"})
		}
	}
	days := s.d.Settings.Int(ctx, "profile.viewers_window_days", 30)
	if days < 1 {
		days = 1
	}
	since := s.today().AddDate(0, 0, -(days - 1)).Format(dateLayout)
	rows, err := s.repo.viewers(ctx, me.ID, since, page.Cursor.Time, page.Cursor.ID, page.Limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > page.Limit {
		rows = rows[:page.Limit]
		last := rows[len(rows)-1]
		next = httpx.EncodeCursor(httpx.Cursor{ID: last.ViewerID, Time: last.Day})
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ViewerID
	}
	cards, err := s.d.Users.Summaries(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	out := []Viewer{}
	list := []userdir.Summary{}
	for _, r := range rows {
		if c, ok := cards[r.ViewerID]; ok {
			list = append(list, c)
			out = append(out, Viewer{ViewedOn: r.Day})
		}
	}
	if err := HideMinorCities(ctx, s.d, me.ID, list); err != nil {
		return nil, "", err
	}
	for i := range out {
		out[i].User = list[i]
	}
	return out, next, nil
}

// ---- stats ----

// numericTypes are summed into career totals.
var numericTypes = []string{"int", "decimal", "duration_ms"}

// Stats is GET /profiles/:userId/stats.
func (s *Service) Stats(ctx context.Context, viewer userdir.Basic, targetID int64) (Stats, error) {
	sj, err := s.load(ctx, viewer, targetID)
	if err != nil {
		return Stats{}, err
	}
	out := Stats{Entries: []StatEntry{}}
	if !sj.visible(sj.row.Visibility) || !sj.visible(sj.row.StatsVisibility) {
		return out, nil
	}
	rows, err := s.repo.listStats(ctx, targetID)
	if err != nil {
		return Stats{}, err
	}
	for _, r := range rows {
		out.Entries = append(out.Entries, toStat(r))
	}
	has, err := s.d.Entitlements.Has(ctx, targetID, entitlements.AdvancedStats)
	if err != nil {
		return Stats{}, err
	}
	if !has {
		return out, nil
	}
	ids := []int64{}
	for _, r := range rows {
		if !slices.Contains(ids, r.TemplateID) {
			ids = append(ids, r.TemplateID)
		}
	}
	tpl, err := s.repo.templatesFields(ctx, ids)
	if err != nil {
		return Stats{}, err
	}
	out.Totals = map[string]float64{}
	for _, e := range out.Entries {
		for _, f := range sport.ParseFields(tpl[e.TemplateID]) {
			if !slices.Contains(numericTypes, f.Type) {
				continue
			}
			if v, ok := e.Stats[f.Key].(float64); ok {
				out.Totals[f.Key] += v
			}
		}
	}
	return out, nil
}

func toStat(r statRow) StatEntry {
	e := StatEntry{ID: r.ID, SportID: r.SportID, TemplateID: r.TemplateID, Season: r.Season, Stats: map[string]any{}, UpdatedAt: r.UpdatedAt.UTC()}
	_ = json.Unmarshal(r.Stats, &e.Stats)
	return e
}

// validateStats checks values against the template fields; errors go in fields["stats.<key>"].
func validateStats(fields map[string]string, tplFields []sport.StatField, in map[string]any) {
	if len(in) == 0 {
		fields["stats"] = "must have at least one value"
		return
	}
	byKey := map[string]sport.StatField{}
	for _, f := range tplFields {
		byKey[f.Key] = f
	}
	for k, v := range in {
		f, ok := byKey[k]
		if !ok {
			fields["stats."+k] = "is not a field of this template"
			continue
		}
		if msg := checkValue(f.Type, v); msg != "" {
			fields["stats."+k] = msg
		}
	}
}

func checkValue(typ string, v any) string {
	if typ == "text" {
		str, ok := v.(string)
		if !ok {
			return "must be text"
		}
		if utf8.RuneCountInString(str) > maxStatText {
			return "is too long"
		}
		return ""
	}
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return "must be a number"
	}
	switch typ {
	case "int":
		if n != math.Trunc(n) {
			return "must be a whole number"
		}
	case "duration_ms":
		if n != math.Trunc(n) || n < 0 {
			return "must be a whole number of milliseconds"
		}
	case "percent":
		if n < 0 || n > 100 {
			return "must be between 0 and 100"
		}
	case "decimal":
	default:
		return "has an unknown field type"
	}
	return ""
}

func checkSeason(fields map[string]string, season string) string {
	season = strings.TrimSpace(season)
	if !seasonRe.MatchString(season) {
		fields["season"] = "must look like 2025 or 2025-26"
	}
	return season
}

// CreateStat is POST /me/stats.
func (s *Service) CreateStat(ctx context.Context, me userdir.Basic, in statCreateBody) (StatEntry, error) {
	fields := map[string]string{}
	if in.SportID <= 0 {
		fields["sportId"] = "is required"
	}
	if in.TemplateID <= 0 {
		fields["templateId"] = "is required"
	}
	season := checkSeason(fields, in.Season)
	if len(fields) > 0 {
		return StatEntry{}, httpx.Validation(fields)
	}
	raw, err := s.repo.templateFields(ctx, s.d.DB, in.TemplateID, in.SportID, true)
	if db.IsNoRows(err) {
		return StatEntry{}, httpx.Validation(map[string]string{"templateId": "is not an active template of this sport"})
	}
	if err != nil {
		return StatEntry{}, err
	}
	validateStats(fields, sport.ParseFields(raw), in.Stats)
	if len(fields) > 0 {
		return StatEntry{}, httpx.Validation(fields)
	}
	b, _ := json.Marshal(in.Stats)
	id, err := s.repo.insertStat(ctx, me.ID, in.SportID, in.TemplateID, season, b)
	if db.IsDuplicate(err) {
		return StatEntry{}, httpx.Conflict("You already have stats for this sport and season.")
	}
	if err != nil {
		return StatEntry{}, err
	}
	return StatEntry{ID: id, SportID: in.SportID, TemplateID: in.TemplateID, Season: season, Stats: in.Stats, UpdatedAt: s.d.Now().UTC()}, nil
}

// UpdateStat is PUT /me/stats/:id (scoped to the caller: another user's id → 404).
func (s *Service) UpdateStat(ctx context.Context, me userdir.Basic, id int64, in statUpdateBody) (StatEntry, error) {
	fields := map[string]string{}
	if in.Season != nil {
		checkSeason(fields, *in.Season)
	}
	if len(fields) > 0 {
		return StatEntry{}, httpx.Validation(fields)
	}
	var out StatEntry
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.stat(ctx, tx, id, me.ID, true)
		if db.IsNoRows(err) {
			return httpx.NotFound("Stats")
		}
		if err != nil {
			return err
		}
		out = toStat(cur)
		if in.Season != nil {
			out.Season = strings.TrimSpace(*in.Season)
		}
		if in.Stats != nil {
			raw, err := s.repo.templateFields(ctx, tx, cur.TemplateID, cur.SportID, false)
			if err != nil && !db.IsNoRows(err) {
				return err
			}
			validateStats(fields, sport.ParseFields(raw), *in.Stats)
			if len(fields) > 0 {
				return httpx.Validation(fields)
			}
			out.Stats = *in.Stats
		}
		b, _ := json.Marshal(out.Stats)
		err = s.repo.updateStat(ctx, tx, id, me.ID, out.Season, b)
		if db.IsDuplicate(err) {
			return httpx.Conflict("You already have stats for this sport and season.")
		}
		return err
	})
	if err != nil {
		return StatEntry{}, err
	}
	out.UpdatedAt = s.d.Now().UTC()
	return out, nil
}

// DeleteStat is DELETE /me/stats/:id.
func (s *Service) DeleteStat(ctx context.Context, me userdir.Basic, id int64) error {
	n, err := s.repo.deleteStat(ctx, id, me.ID)
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.NotFound("Stats")
	}
	return nil
}

// ---- achievements ----

// Achievements is GET /profiles/:userId/achievements.
func (s *Service) Achievements(ctx context.Context, viewer userdir.Basic, targetID int64) ([]Achievement, error) {
	sj, err := s.load(ctx, viewer, targetID)
	if err != nil {
		return nil, err
	}
	if !sj.visible(sj.row.Visibility) {
		return []Achievement{}, nil
	}
	rows, err := s.repo.listAchievements(ctx, targetID)
	if err != nil {
		return nil, err
	}
	return s.toAchievements(ctx, rows), nil
}

func (s *Service) toAchievements(ctx context.Context, rows []achievementRow) []Achievement {
	ids := []int64{}
	for _, r := range rows {
		if r.CertID.Valid {
			ids = append(ids, r.CertID.Int64)
		}
	}
	urls := s.d.Media.ViewURLs(ctx, ids)
	out := make([]Achievement, 0, len(rows))
	for _, r := range rows {
		a := Achievement{ID: r.ID, Title: r.Title, Level: r.Level, Description: strPtr(r.Description), CreatedAt: r.CreatedAt.UTC()}
		if r.SportID.Valid {
			a.SportID = &r.SportID.Int64
		}
		if r.AchievedOn.Valid {
			d := r.AchievedOn.Time.Format(dateLayout)
			a.AchievedOn = &d
		}
		if r.CertID.Valid {
			a.CertificateUploadID = &r.CertID.Int64
			if u, ok := urls[r.CertID.Int64]; ok {
				a.CertificateURL = &u
			}
		}
		out = append(out, a)
	}
	return out
}

// achievementInput merges in over cur (nil for create) and validates it.
func (s *Service) achievementInput(fields map[string]string, in achievementBody, cur *achievementValues) achievementValues {
	v := achievementValues{Level: "other"}
	if cur != nil {
		v = *cur
	}
	if in.Title != nil || cur == nil {
		t := ""
		if in.Title != nil {
			t = *in.Title
		}
		v.Title = text(fields, "title", t, 1, 120)
	}
	if in.Description != nil {
		v.Description = optText(fields, "description", in.Description, 500)
	}
	if in.Level != nil {
		if !slices.Contains(Levels, *in.Level) {
			fields["level"] = "must be one of " + strings.Join(Levels, ", ")
		}
		v.Level = *in.Level
	}
	if in.AchievedOn != nil {
		v.AchievedOn = strings.TrimSpace(*in.AchievedOn)
		if v.AchievedOn != "" {
			t, err := time.Parse(dateLayout, v.AchievedOn)
			if err != nil {
				fields["achievedOn"] = "must be a date (YYYY-MM-DD)"
			} else if t.After(s.today()) {
				fields["achievedOn"] = "cannot be in the future"
			}
		}
	}
	if in.SportID != nil {
		if *in.SportID < 0 {
			fields["sportId"] = "is invalid"
		}
		v.SportID = in.SportID
		if *in.SportID == 0 {
			v.SportID = nil
		}
	}
	if in.CertificateUploadID != nil {
		if *in.CertificateUploadID < 0 {
			fields["certificateUploadId"] = "is invalid"
		}
		v.CertID = in.CertificateUploadID
		if *in.CertificateUploadID == 0 {
			v.CertID = nil
		}
	}
	return v
}

// saveAchievementRefs validates the sport and claims a new certificate inside tx.
func (s *Service) saveAchievementRefs(ctx context.Context, tx *sql.Tx, userID int64, in achievementBody, v achievementValues, oldCert int64) error {
	if in.SportID != nil && v.SportID != nil {
		fields := map[string]string{}
		if err := s.checkSport(ctx, tx, fields, "sportId", "sportId", *v.SportID, nil); err != nil {
			return err
		}
		if len(fields) > 0 {
			return httpx.Validation(fields)
		}
	}
	if v.CertID != nil && *v.CertID != oldCert {
		if err := s.d.Media.Claim(ctx, tx, userID, *v.CertID, pmedia.PurposeAchievementCert); err != nil {
			return claimErr(err, "certificateUploadId")
		}
	}
	return nil
}

// CreateAchievement is POST /me/achievements (max profile.max_achievements).
func (s *Service) CreateAchievement(ctx context.Context, me userdir.Basic, in achievementBody) (Achievement, error) {
	fields := map[string]string{}
	v := s.achievementInput(fields, in, nil)
	if len(fields) > 0 {
		return Achievement{}, httpx.Validation(fields)
	}
	max := s.d.Settings.Int(ctx, "profile.max_achievements", 50)
	var id int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if err := s.repo.lockProfile(ctx, tx, me.ID); db.IsNoRows(err) {
			return httpx.NotFound("Profile")
		} else if err != nil {
			return err
		}
		n, err := s.repo.countAchievements(ctx, tx, me.ID)
		if err != nil {
			return err
		}
		if n >= max {
			return httpx.LimitReached("You have reached the maximum number of achievements.")
		}
		if err := s.saveAchievementRefs(ctx, tx, me.ID, in, v, 0); err != nil {
			return err
		}
		id, err = s.repo.insertAchievement(ctx, tx, me.ID, v)
		return err
	})
	if err != nil {
		return Achievement{}, err
	}
	return s.oneAchievement(ctx, me.ID, id)
}

func (s *Service) oneAchievement(ctx context.Context, userID, id int64) (Achievement, error) {
	r, err := s.repo.achievement(ctx, s.d.DB, id, userID, false)
	if db.IsNoRows(err) {
		return Achievement{}, httpx.NotFound("Achievement")
	}
	if err != nil {
		return Achievement{}, err
	}
	return s.toAchievements(ctx, []achievementRow{r})[0], nil
}

func valuesOf(r achievementRow) achievementValues {
	v := achievementValues{Title: r.Title, Description: r.Description.String, Level: r.Level}
	if r.SportID.Valid {
		v.SportID = &r.SportID.Int64
	}
	if r.AchievedOn.Valid {
		v.AchievedOn = r.AchievedOn.Time.Format(dateLayout)
	}
	if r.CertID.Valid {
		v.CertID = &r.CertID.Int64
	}
	return v
}

// UpdateAchievement is PUT /me/achievements/:id (partial; another user's id → 404).
func (s *Service) UpdateAchievement(ctx context.Context, me userdir.Basic, id int64, in achievementBody) (Achievement, error) {
	var oldCert int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.achievement(ctx, tx, id, me.ID, true)
		if db.IsNoRows(err) {
			return httpx.NotFound("Achievement")
		}
		if err != nil {
			return err
		}
		fields := map[string]string{}
		curV := valuesOf(cur)
		v := s.achievementInput(fields, in, &curV)
		if len(fields) > 0 {
			return httpx.Validation(fields)
		}
		if err := s.saveAchievementRefs(ctx, tx, me.ID, in, v, cur.CertID.Int64); err != nil {
			return err
		}
		if cur.CertID.Valid && (v.CertID == nil || *v.CertID != cur.CertID.Int64) {
			oldCert = cur.CertID.Int64
		}
		return s.repo.updateAchievement(ctx, tx, id, me.ID, v)
	})
	if err != nil {
		return Achievement{}, err
	}
	if oldCert != 0 {
		s.purge(ctx, me.ID, oldCert)
	}
	return s.oneAchievement(ctx, me.ID, id)
}

// DeleteAchievement is DELETE /me/achievements/:id; the certificate file is purged.
func (s *Service) DeleteAchievement(ctx context.Context, me userdir.Basic, id int64) error {
	var cert int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.achievement(ctx, tx, id, me.ID, true)
		if db.IsNoRows(err) {
			return httpx.NotFound("Achievement")
		}
		if err != nil {
			return err
		}
		cert = cur.CertID.Int64
		return s.repo.deleteAchievement(ctx, tx, id, me.ID)
	})
	if err != nil {
		return err
	}
	if cert != 0 {
		s.purge(ctx, me.ID, cert)
	}
	return nil
}

// ---- events ----

// onUserDeleted clears the profile data of a deleted user and purges its files (idempotent).
func (s *Service) onUserDeleted(ctx context.Context, e events.Event) error {
	var p events.UserPayload
	if err := e.Decode(&p); err != nil || p.UserID == 0 {
		return nil // malformed: nothing to retry
	}
	// Files first: Purge is idempotent, so a retry after a failed wipe still finds the ids.
	uploads, err := s.repo.uploadsOf(ctx, s.d.DB, p.UserID)
	if err != nil {
		return err
	}
	for _, id := range uploads {
		if err := s.d.Media.Purge(ctx, id); err != nil {
			return err
		}
	}
	if err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error { return s.repo.wipe(ctx, tx, p.UserID) }); err != nil {
		return err
	}
	s.d.Users.Invalidate(p.UserID)
	logger.From(ctx).Info("profile data wiped", slog.Int64("userId", p.UserID), slog.Int("uploads", len(uploads)))
	return nil
}

func strPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func intPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

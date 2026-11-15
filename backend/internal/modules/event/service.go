package event

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/eligibility"
	"athlix/backend/internal/platform/entitlements"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/media"
	"athlix/backend/internal/platform/requirements"
	"athlix/backend/internal/platform/userdir"
)

// Service holds the event rules.
type Service struct {
	d *app.Deps
	r *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, r: &repo{db: d.DB}} }

func (s *Service) now() time.Time { return s.d.Now().UTC() }

var (
	eventTypes        = set("tournament", "trial", "camp", "match", "other")
	participantTypes  = set("individual", "team")
	competitionLevels = set("open", "school", "college", "club", "district", "state", "national", "other")
	genderCategories  = set("open", "male", "female", "mixed")
	categories        = set("government", "private")
	paymentModes      = set("cash", "upi", "bank_transfer", "online_link")
	statuses          = set(stDraft, stPendingPayment, stPendingReview, stPublished, stRejected, stCancelled, stCompleted, stRemoved)

	pincodeRe = regexp.MustCompile(`^[1-9][0-9]{5}$`)
	phoneRe   = regexp.MustCompile(`^[6-9][0-9]{9}$`)
)

func set(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range v {
		m[s] = true
	}
	return m
}

// normPhone accepts 10 digits with an optional +91 / 91 / 0 prefix and spaces or dashes.
func normPhone(p string) (string, bool) {
	p = strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(p))
	switch {
	case strings.HasPrefix(p, "+91"):
		p = p[3:]
	case len(p) == 12 && strings.HasPrefix(p, "91"):
		p = p[2:]
	case len(p) == 11 && strings.HasPrefix(p, "0"):
		p = p[1:]
	}
	return p, phoneRe.MatchString(p)
}

func optStr(o Opt[string]) *string {
	if !o.Valid {
		return nil
	}
	v := strings.TrimSpace(o.V)
	if v == "" {
		return nil
	}
	return &v
}

func optInt(o Opt[int]) *int {
	if !o.Valid {
		return nil
	}
	v := o.V
	return &v
}

func optFloat(o Opt[float64]) *float64 {
	if !o.Valid {
		return nil
	}
	v := o.V
	return &v
}

func runeLen(s *string) int {
	if s == nil {
		return 0
	}
	return len([]rune(*s))
}

// merge applies the Set fields of in onto r and checks per-field shapes.
func (s *Service) merge(ctx context.Context, r *rec, in Input, fields map[string]string) {
	str := func(o Opt[string], dst *string) {
		if o.Set {
			*dst = ""
			if v := optStr(o); v != nil {
				*dst = *v
			}
		}
	}
	nstr := func(o Opt[string], dst **string) {
		if o.Set {
			*dst = optStr(o)
		}
	}
	enum := func(o Opt[string], dst *string, allowed map[string]bool, key string) {
		if !o.Set {
			return
		}
		if !o.Valid || !allowed[o.V] {
			fields[key] = "is not a valid value"
			return
		}
		*dst = o.V
	}
	if in.SportID.Set {
		r.SportID = in.SportID.V
	}
	str(in.Title, &r.Title)
	nstr(in.Description, &r.Description)
	enum(in.EventType, &r.EventType, eventTypes, "eventType")
	enum(in.ParticipantType, &r.ParticipantType, participantTypes, "participantType")
	if in.TeamSizeMin.Set {
		r.TeamSizeMin = optInt(in.TeamSizeMin)
	}
	if in.TeamSizeMax.Set {
		r.TeamSizeMax = optInt(in.TeamSizeMax)
	}
	enum(in.CompetitionLevel, &r.CompetitionLevel, competitionLevels, "competitionLevel")
	enum(in.Category, &r.Category, categories, "category")
	if in.Eligibility.Set {
		el := EligibilityInput{Type: eligibility.TypeOpen}
		if in.Eligibility.Valid {
			el = in.Eligibility.V
		}
		rule, f := s.buildRule(ctx, el)
		for k, v := range f {
			fields[k] = v
		}
		if len(f) == 0 {
			setRule(r, rule)
		}
	}
	enum(in.GenderCategory, &r.GenderCategory, genderCategories, "genderCategory")
	if in.AgeMin.Set {
		r.AgeMin = optInt(in.AgeMin)
	}
	if in.AgeMax.Set {
		r.AgeMax = optInt(in.AgeMax)
	}
	str(in.VenueName, &r.VenueName)
	nstr(in.AddressLine, &r.AddressLine)
	str(in.City, &r.City)
	nstr(in.District, &r.District)
	str(in.State, &r.State)
	nstr(in.Pincode, &r.Pincode)
	if in.Latitude.Set {
		r.Latitude = optFloat(in.Latitude)
	}
	if in.Longitude.Set {
		r.Longitude = optFloat(in.Longitude)
	}
	tm := func(o Opt[time.Time], dst *time.Time) {
		if o.Set {
			*dst = time.Time{}
			if o.Valid {
				*dst = o.V.UTC()
			}
		}
	}
	tm(in.StartsAt, &r.StartsAt)
	tm(in.EndsAt, &r.EndsAt)
	tm(in.RegistrationDeadline, &r.Deadline)
	if in.EntryFeePaise.Set {
		r.EntryFeePaise = in.EntryFeePaise.V
	}
	if in.PaymentModes.Set {
		r.PaymentModes = []string{}
		seen := map[string]bool{}
		for _, m := range in.PaymentModes.V {
			if !paymentModes[m] {
				fields["paymentModes"] = "has an unknown payment mode"
				continue
			}
			if !seen[m] {
				seen[m] = true
				r.PaymentModes = append(r.PaymentModes, m)
			}
		}
	}
	nstr(in.PaymentInstructions, &r.PaymentInstructions)
	if in.MaxSlots.Set {
		r.MaxSlots = in.MaxSlots.V
	}
	nstr(in.Rules, &r.Rules)
	nstr(in.PrizeInfo, &r.PrizeInfo)
	str(in.ContactName, &r.ContactName)
	if in.ContactPhone.Set {
		p, ok := normPhone(in.ContactPhone.V)
		if !in.ContactPhone.Valid || !ok {
			fields["contactPhone"] = "must be a 10-digit Indian mobile number"
		} else {
			r.phone, r.PhoneLast4 = p, crypto.Last4(p)
		}
	}
	if in.BannerUploadID.Set {
		r.BannerUploadID = nil
		if in.BannerUploadID.Valid && in.BannerUploadID.V > 0 {
			id := in.BannerUploadID.V
			r.BannerUploadID = &id
		}
	}
}

// validate checks the merged row (cross-field rules and config limits).
func (s *Service) validate(ctx context.Context, r *rec, checkDates bool, fields map[string]string) {
	need := func(key string, ok bool) {
		if !ok && fields[key] == "" {
			fields[key] = "is required"
		}
	}
	maxLen := func(key string, v *string, n int) {
		if runeLen(v) > n && fields[key] == "" {
			fields[key] = "is too long (max " + strconv.Itoa(n) + ")"
		}
	}
	need("sportId", r.SportID > 0)
	if n := len([]rune(r.Title)); n < 3 || n > 120 {
		if n == 0 {
			need("title", false)
		} else {
			fields["title"] = "must be 3 to 120 characters"
		}
	}
	maxLen("description", r.Description, 5000)
	maxLen("rules", r.Rules, 5000)
	maxLen("prizeInfo", r.PrizeInfo, 1000)
	maxLen("paymentInstructions", r.PaymentInstructions, 500)
	maxLen("addressLine", r.AddressLine, 255)
	maxLen("district", r.District, 64)
	need("venueName", r.VenueName != "")
	maxLen("venueName", &r.VenueName, 120)
	need("city", r.City != "")
	maxLen("city", &r.City, 64)
	need("state", r.State != "")
	maxLen("state", &r.State, 64)
	need("contactName", r.ContactName != "")
	maxLen("contactName", &r.ContactName, 80)
	need("contactPhone", r.PhoneLast4 != "")
	if r.Pincode != nil && !pincodeRe.MatchString(*r.Pincode) {
		fields["pincode"] = "must be a 6-digit PIN code"
	}
	if (r.Latitude == nil) != (r.Longitude == nil) {
		fields["latitude"] = "latitude and longitude go together"
	}
	if r.Latitude != nil && (*r.Latitude < -90 || *r.Latitude > 90) {
		fields["latitude"] = "must be between -90 and 90"
	}
	if r.Longitude != nil && (*r.Longitude < -180 || *r.Longitude > 180) {
		fields["longitude"] = "must be between -180 and 180"
	}
	if r.ParticipantType == "team" {
		switch {
		case r.TeamSizeMin == nil || r.TeamSizeMax == nil:
			fields["teamSizeMin"] = "team size is required for team events"
		case *r.TeamSizeMin < 1 || *r.TeamSizeMax > maxTeamMembers || *r.TeamSizeMin > *r.TeamSizeMax:
			fields["teamSizeMin"] = "must satisfy 1 ≤ teamSizeMin ≤ teamSizeMax ≤ " + strconv.Itoa(maxTeamMembers)
		}
	} else {
		r.TeamSizeMin, r.TeamSizeMax = nil, nil
	}
	if r.AgeMin != nil && (*r.AgeMin < 1 || *r.AgeMin > 100) {
		fields["ageMin"] = "must be between 1 and 100"
	}
	if r.AgeMax != nil && (*r.AgeMax < 1 || *r.AgeMax > 100) {
		fields["ageMax"] = "must be between 1 and 100"
	}
	if r.AgeMin != nil && r.AgeMax != nil && *r.AgeMin > *r.AgeMax {
		fields["ageMax"] = "must be at least ageMin"
	}
	if limit := s.d.Settings.Int(ctx, "event.max_slots_limit", defMaxSlotsLimit); r.MaxSlots < 1 || r.MaxSlots > limit {
		fields["maxSlots"] = "must be between 1 and " + strconv.Itoa(limit)
	}
	if r.EntryFeePaise < 0 || r.EntryFeePaise > math.MaxUint32 {
		fields["entryFeePaise"] = "is out of range"
	} else if r.EntryFeePaise == 0 && !s.d.Settings.Bool(ctx, "event.allow_free_events", defAllowFree) {
		fields["entryFeePaise"] = "free events are not allowed"
	}
	if r.EntryFeePaise > 0 && len(r.PaymentModes) == 0 && fields["paymentModes"] == "" {
		fields["paymentModes"] = "pick at least one payment mode for a paid event"
	}
	if checkDates {
		need("startsAt", !r.StartsAt.IsZero())
		need("endsAt", !r.EndsAt.IsZero())
		need("registrationDeadline", !r.Deadline.IsZero())
		if !r.StartsAt.IsZero() {
			h := s.d.Settings.Int(ctx, "event.min_hours_before_start", defMinHoursBefore)
			if r.StartsAt.Before(s.now().Add(time.Duration(h) * time.Hour)) {
				fields["startsAt"] = "must be at least " + strconv.Itoa(h) + " hours from now"
			}
			if !r.EndsAt.IsZero() && r.EndsAt.Before(r.StartsAt) {
				fields["endsAt"] = "must not be before startsAt"
			}
			if !r.Deadline.IsZero() && r.Deadline.After(r.StartsAt) {
				fields["registrationDeadline"] = "must not be after startsAt"
			}
		}
	}
}

// lockedChanges lists fields a published event may not change.
func lockedChanges(a, b rec) []string {
	var out []string
	eqI := func(x, y *int) bool { return (x == nil && y == nil) || (x != nil && y != nil && *x == *y) }
	eqI64 := func(x, y *int64) bool { return (x == nil && y == nil) || (x != nil && y != nil && *x == *y) }
	eqF := func(x, y *float64) bool { return (x == nil && y == nil) || (x != nil && y != nil && *x == *y) }
	eqS := func(x, y *string) bool { return (x == nil && y == nil) || (x != nil && y != nil && *x == *y) }
	chk := func(name string, same bool) {
		if !same {
			out = append(out, name)
		}
	}
	chk("sportId", a.SportID == b.SportID)
	chk("title", a.Title == b.Title)
	chk("eventType", a.EventType == b.EventType)
	chk("participantType", a.ParticipantType == b.ParticipantType)
	chk("teamSizeMin", eqI(a.TeamSizeMin, b.TeamSizeMin))
	chk("teamSizeMax", eqI(a.TeamSizeMax, b.TeamSizeMax))
	chk("competitionLevel", a.CompetitionLevel == b.CompetitionLevel)
	chk("category", a.Category == b.Category)
	chk("eligibility", a.ElType == b.ElType && eqS(a.ElScope, b.ElScope) && eqI64(a.ElDistrictID, b.ElDistrictID) &&
		eqI64(a.ElStateID, b.ElStateID) && a.RequireVerified == b.RequireVerified)
	chk("genderCategory", a.GenderCategory == b.GenderCategory)
	chk("ageMin", eqI(a.AgeMin, b.AgeMin))
	chk("ageMax", eqI(a.AgeMax, b.AgeMax))
	chk("city", a.City == b.City)
	chk("district", eqS(a.District, b.District))
	chk("state", a.State == b.State)
	chk("pincode", eqS(a.Pincode, b.Pincode))
	chk("latitude", eqF(a.Latitude, b.Latitude))
	chk("longitude", eqF(a.Longitude, b.Longitude))
	chk("startsAt", a.StartsAt.Equal(b.StartsAt))
	chk("endsAt", a.EndsAt.Equal(b.EndsAt))
	chk("registrationDeadline", a.Deadline.Equal(b.Deadline))
	chk("entryFeePaise", a.EntryFeePaise == b.EntryFeePaise)
	chk("maxSlots", a.MaxSlots == b.MaxSlots)
	chk("paymentModes", strings.Join(a.PaymentModes, ",") == strings.Join(b.PaymentModes, ","))
	return out
}

func (s *Service) sportCheck(ctx context.Context, r rec, fields map[string]string) error {
	if r.SportID <= 0 || fields["sportId"] != "" {
		return nil
	}
	ok, err := s.r.sportActive(ctx, r.SportID)
	if err != nil {
		return err
	}
	if !ok {
		fields["sportId"] = "is not an active sport"
	}
	return nil
}

// claimBanner attaches a new banner upload in tx (422 when it is not the caller's banner).
func (s *Service) claimBanner(ctx context.Context, tx *sql.Tx, uid int64, prev, next *int64) error {
	if next == nil || (prev != nil && *prev == *next) {
		return nil
	}
	if err := s.d.Media.Claim(ctx, tx, uid, *next, media.PurposeEventBanner); err != nil {
		if errors.Is(err, media.ErrNotFound) {
			return httpx.Validation(map[string]string{"bannerUploadId": "is not your event banner upload"})
		}
		return err
	}
	return nil
}

func (s *Service) encryptPhone(r rec) ([]byte, error) {
	if r.phone == "" {
		return nil, nil
	}
	return s.d.Crypto.EncryptString(r.phone)
}

// Create is POST /events.
func (s *Service) Create(ctx context.Context, u userdir.Basic, in Input) (Event, error) {
	if !u.HasRole("organiser") {
		return Event{}, httpx.Forbidden("Only organisers can create events.")
	}
	if s.d.Requirements.Required(ctx, "organiser", requirements.OrganiserApproval, requirements.GateCreateEvent) {
		ok, err := s.r.organiserApproved(ctx, u.ID)
		if err != nil {
			return Event{}, err
		}
		if !ok {
			return Event{}, httpx.Coded(403, httpx.CodeOrganiserPending, "Your organiser profile must be approved before you create events.")
		}
	}
	r := rec{OrganiserID: u.ID, EventType: "tournament", ParticipantType: "individual", CompetitionLevel: "open",
		Category: "private", ElType: eligibility.TypeOpen, RequireVerified: true, GenderCategory: "open",
		PaymentModes: []string{}, Status: stDraft}
	fields := map[string]string{}
	s.merge(ctx, &r, in, fields)
	s.validate(ctx, &r, true, fields)
	if err := s.sportCheck(ctx, r, fields); err != nil {
		return Event{}, err
	}
	if len(fields) > 0 {
		return Event{}, httpx.Validation(fields)
	}
	enc, err := s.encryptPhone(r)
	if err != nil {
		return Event{}, err
	}
	var id int64
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		n, err := s.r.countActive(ctx, tx, u.ID)
		if err != nil {
			return err
		}
		if max := s.d.Settings.Int(ctx, "event.max_active_per_organiser", defMaxActive); n >= max {
			return httpx.LimitReached("You already have the maximum number of active events.")
		}
		if err := s.claimBanner(ctx, tx, u.ID, nil, r.BannerUploadID); err != nil {
			return err
		}
		id, err = s.r.insert(ctx, tx, r, enc, s.d.Crypto.KeyVersion())
		return err
	})
	if err != nil {
		return Event{}, err
	}
	return s.Get(ctx, u.ID, id)
}

// owned loads an event of the caller (404 when it is not theirs).
func (s *Service) owned(ctx context.Context, q db.Querier, uid, id int64, forUpdate bool) (rec, error) {
	r, err := s.r.get(ctx, q, id, forUpdate)
	if err != nil {
		return rec{}, err
	}
	if r.OrganiserID != uid {
		return rec{}, httpx.NotFound("event")
	}
	return r, nil
}

// Update is PUT /events/:id.
func (s *Service) Update(ctx context.Context, uid, id int64, in Input) (Event, error) {
	var before rec
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		before, err = s.owned(ctx, tx, uid, id, true)
		if err != nil {
			return err
		}
		to := before.Status
		switch before.Status {
		case stDraft, stRejected, stPublished:
		case stPendingPayment:
			to = stDraft // the open listing order is expired by listingpayment on event.updated
		case stPendingReview:
			return httpx.InvalidState("The event is in review and cannot be edited.")
		default:
			return httpx.InvalidState("This event can no longer be edited.")
		}
		r := before
		fields := map[string]string{}
		s.merge(ctx, &r, in, fields)
		published := before.Status == stPublished
		s.validate(ctx, &r, !published, fields)
		if published {
			for _, f := range lockedChanges(before, r) {
				fields[f] = "is locked once the event is published"
			}
		}
		if err := s.sportCheck(ctx, r, fields); err != nil {
			return err
		}
		if len(fields) > 0 {
			return httpx.Validation(fields)
		}
		if err := s.claimBanner(ctx, tx, uid, before.BannerUploadID, r.BannerUploadID); err != nil {
			return err
		}
		enc, err := s.encryptPhone(r)
		if err != nil {
			return err
		}
		return s.r.update(ctx, tx, r, enc, s.d.Crypto.KeyVersion(), before.Status, to)
	})
	if err != nil {
		return Event{}, err
	}
	if before.Status == stPublished || before.Status == stPendingPayment {
		s.publish(ctx, events.EventUpdated, uid, before, "")
	}
	return s.Get(ctx, uid, id)
}

func (s *Service) publish(ctx context.Context, typ string, actor int64, r rec, reason string) {
	err := s.d.Events.Publish(ctx, events.New(typ, actor, events.EventPayload{EventID: r.ID, OrganiserID: r.OrganiserID, SportID: r.SportID, Reason: reason}))
	if err != nil {
		logger.From(ctx).Error("publish event", slog.String("type", typ), slog.Int64("eventId", r.ID), slog.String("err", err.Error()))
	}
}

// visible applies the read rule for a non-admin caller.
func (s *Service) visible(ctx context.Context, uid int64, r rec) error {
	if r.OrganiserID == uid {
		return nil
	}
	switch r.Status {
	case stPublished, stCompleted:
	case stCancelled:
		reg, err := s.r.myRegistration(ctx, r.ID, uid)
		if err != nil {
			return err
		}
		if reg == nil {
			return httpx.NotFound("event")
		}
	default:
		return httpx.NotFound("event")
	}
	blocked, err := s.d.Users.BlockedEitherWay(ctx, uid, r.OrganiserID)
	if err != nil {
		return err
	}
	if blocked {
		return httpx.NotFound("event")
	}
	return nil
}

// Get is GET /events/:id.
func (s *Service) Get(ctx context.Context, uid, id int64) (Event, error) {
	r, err := s.r.get(ctx, nil, id, false)
	if err != nil {
		return Event{}, err
	}
	if err := s.visible(ctx, uid, r); err != nil {
		return Event{}, err
	}
	return s.detail(ctx, uid, r, false)
}

// detail builds Event. admin=true skips the caller-specific parts.
func (s *Service) detail(ctx context.Context, uid int64, r rec, admin bool) (Event, error) {
	v := s.newViewer(ctx, uid)
	sums, err := s.summaries(ctx, v, []rec{r}, admin)
	if err != nil {
		return Event{}, err
	}
	ev := Event{Summary: sums[0], Description: r.Description, TeamSizeMin: r.TeamSizeMin, TeamSizeMax: r.TeamSizeMax,
		AddressLine: r.AddressLine, Pincode: r.Pincode, Latitude: r.Latitude, Longitude: r.Longitude,
		PaymentModes: r.PaymentModes, PaymentInstructions: r.PaymentInstructions, Rules: r.Rules, PrizeInfo: r.PrizeInfo,
		ContactName: r.ContactName, ContactPhoneMasked: crypto.MaskMobile(r.PhoneLast4), RejectionReason: r.RejectionReason,
		CancelReason: r.CancelReason, PublishedAt: r.PublishedAt, IsOwner: !admin && r.OrganiserID == uid}
	if !admin && !ev.IsOwner {
		if ev.MyEligibility != nil && !ev.MyEligibility.Eligible {
			ev.PaymentInstructions = nil
		}
		reg, err := s.r.myRegistration(ctx, r.ID, uid)
		if err != nil {
			return Event{}, err
		}
		if reg != nil {
			s.fillQualified(ctx, reg)
		}
		ev.MyRegistration = reg
	}
	if ev.HasGroupChat, err = s.r.hasGroupChat(ctx, r.ID); err != nil {
		return Event{}, err
	}
	if ev.IsOwner {
		ls, err := s.r.latestListing(ctx, []int64{r.ID})
		if err != nil {
			return Event{}, err
		}
		if l, ok := ls[r.ID]; ok {
			ev.Listing = &l
		}
	}
	return ev, nil
}

func (s *Service) fillQualified(ctx context.Context, g *Registration) {
	if g.qualifiedDistrictID == nil {
		return
	}
	if d, ok := s.d.Eligibility.Places().District(ctx, *g.qualifiedDistrictID); ok {
		g.QualifiedDistrict = &District{ID: d.ID, StateID: d.StateID, Name: d.Name}
	}
}

// summaries builds EventSummary for rows (one users query, one media query, at most one
// identities load). admin=true leaves myEligibility null.
func (s *Service) summaries(ctx context.Context, v *viewer, rs []rec, admin bool) ([]Summary, error) {
	out := make([]Summary, 0, len(rs))
	if len(rs) == 0 {
		return out, nil
	}
	orgIDs, banners := []int64{}, []int64{}
	seen := map[int64]bool{}
	for _, r := range rs {
		if !seen[r.OrganiserID] {
			seen[r.OrganiserID] = true
			orgIDs = append(orgIDs, r.OrganiserID)
		}
		if r.BannerUploadID != nil {
			banners = append(banners, *r.BannerUploadID)
		}
	}
	orgs, err := s.d.Users.Summaries(ctx, orgIDs)
	if err != nil {
		return nil, err
	}
	urls := s.d.Media.ViewURLs(ctx, banners)
	now := s.now()
	for _, r := range rs {
		sm := Summary{ID: r.ID, Title: r.Title, Sport: SportRef{ID: r.SportID, Code: r.SportCode, Name: r.SportName},
			EventType: r.EventType, ParticipantType: r.ParticipantType, CompetitionLevel: r.CompetitionLevel,
			GenderCategory: r.GenderCategory, AgeMin: r.AgeMin, AgeMax: r.AgeMax, VenueName: r.VenueName, City: r.City,
			District: r.District, State: r.State, StartsAt: r.StartsAt, EndsAt: r.EndsAt, RegistrationDeadline: r.Deadline,
			EntryFeePaise: r.EntryFeePaise, MaxSlots: r.MaxSlots, RegistrationsCount: r.RegistrationsCount,
			SlotsLeft: max(r.MaxSlots-r.RegistrationsCount, 0), Status: r.Status, IsFeatured: r.FeaturedAt != nil,
			Category: r.Category, Eligibility: s.ruleView(ctx, r)}
		if o, ok := orgs[r.OrganiserID]; ok {
			o := o
			sm.Organiser = &o
		}
		if r.BannerUploadID != nil {
			if u, ok := urls[*r.BannerUploadID]; ok {
				sm.BannerURL = &u
			}
		}
		if r.dist != nil {
			d := math.Round(*r.dist*10) / 10
			sm.DistanceKm = &d
		}
		if !admin {
			res, err := s.evaluate(ctx, v, r, now)
			if err != nil {
				return nil, err
			}
			sm.MyEligibility = s.myEligibility(ctx, res)
		}
		out = append(out, sm)
	}
	return out, nil
}

// List is GET /events.
func (s *Service) List(ctx context.Context, uid int64, f ListFilter, pg httpx.PageParams) ([]Summary, string, error) {
	now := s.now()
	var by *int
	gender := ""
	if f.EligibleForMe {
		var err error
		if by, gender, err = s.r.profileFacts(ctx, uid); err != nil {
			return nil, "", err
		}
	}
	v := s.newViewer(ctx, uid)
	var cond string
	var args []any
	if !f.ShowAll {
		var err error
		if cond, args, err = s.feedFilter(ctx, v, now); err != nil {
			return nil, "", err
		}
	}
	rs, err := s.r.list(ctx, uid, f, pg, now, by, gender, cond, args)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rs) > pg.Limit {
		rs = rs[:pg.Limit]
		last := rs[len(rs)-1]
		c := httpx.Cursor{ID: last.ID, Time: last.StartsAt.UTC().Format(time.RFC3339Nano)}
		if f.SortDistance && last.dist != nil {
			c.Time = strconv.FormatFloat(*last.dist, 'g', -1, 64)
		}
		next = httpx.EncodeCursor(c)
	}
	sums, err := s.summaries(ctx, v, rs, false)
	return sums, next, err
}

// Featured is GET /events/featured.
func (s *Service) Featured(ctx context.Context, uid, sportID int64, state string, showAll bool) ([]Summary, error) {
	now := s.now()
	v := s.newViewer(ctx, uid)
	var cond string
	var args []any
	if !showAll {
		var err error
		if cond, args, err = s.feedFilter(ctx, v, now); err != nil {
			return nil, err
		}
	}
	n := s.d.Settings.Int(ctx, "event.featured_list_size", defFeaturedListSize)
	rs, err := s.r.featured(ctx, uid, sportID, state, n, now, cond, args)
	if err != nil {
		return nil, err
	}
	return s.summaries(ctx, v, rs, false)
}

// Mine is GET /me/events.
func (s *Service) Mine(ctx context.Context, uid int64, status string, pg httpx.PageParams) ([]Summary, string, error) {
	if status != "" && !statuses[status] {
		return nil, "", httpx.Validation(map[string]string{"status": "is not a valid status"})
	}
	rs, err := s.r.mine(ctx, uid, status, pg)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rs) > pg.Limit {
		rs = rs[:pg.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rs[len(rs)-1].ID})
	}
	sums, err := s.summaries(ctx, s.newViewer(ctx, uid), rs, false)
	return sums, next, err
}

// Contact is GET /events/:id/contact.
func (s *Service) Contact(ctx context.Context, uid, id int64) (Contact, error) {
	r, err := s.r.get(ctx, nil, id, false)
	if err != nil {
		return Contact{}, err
	}
	if r.OrganiserID != uid {
		if err := s.visible(ctx, uid, r); err != nil {
			return Contact{}, err
		}
		if r.Status != stPublished {
			return Contact{}, httpx.Forbidden("The organiser contact is not available for this event.")
		}
		mode := s.d.Settings.String(ctx, "event.show_contact", defShowContact)
		allowed := mode == "always"
		if mode == "after_registration" {
			reg, err := s.r.myRegistration(ctx, r.ID, uid)
			if err != nil {
				return Contact{}, err
			}
			allowed = reg != nil && (reg.Status == "registered" || reg.Status == "waitlisted")
		}
		if !allowed {
			return Contact{}, httpx.Forbidden("Register for the event to see the organiser contact.")
		}
		// Eligibility is re-checked right before payment details are revealed.
		res, err := s.evaluate(ctx, s.newViewer(ctx, uid), r, s.now())
		if err != nil {
			return Contact{}, err
		}
		if !res.Eligible {
			return Contact{}, httpx.NotEligible(res.Reason, res.Action)
		}
		limit := s.d.Settings.Int(ctx, "event.contact_reveals_per_day", defContactReveals)
		ok, err := s.d.Limiter.Allow(ctx, "contact:"+strconv.FormatInt(uid, 10), limit, 24*time.Hour)
		if err != nil {
			return Contact{}, err
		}
		if !ok {
			return Contact{}, httpx.RateLimited("Too many contact reveals today. Try again tomorrow.")
		}
	}
	enc, err := s.r.phoneEnc(ctx, r.ID)
	if err != nil {
		return Contact{}, err
	}
	phone, err := s.d.Crypto.DecryptString(enc)
	if err != nil {
		return Contact{}, err
	}
	return Contact{ContactName: r.ContactName, ContactPhone: "+91" + phone, PaymentModes: r.PaymentModes,
		PaymentInstructions: r.PaymentInstructions}, nil
}

// Cancel is POST /events/:id/cancel.
func (s *Service) Cancel(ctx context.Context, uid, id int64, reason string) (Event, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 500 {
		return Event{}, httpx.Validation(map[string]string{"reason": "is required (max 500 characters)"})
	}
	var r rec
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		if r, err = s.owned(ctx, tx, uid, id, true); err != nil {
			return err
		}
		return s.r.transition(ctx, tx, id, []string{stDraft, stPendingPayment, stPendingReview, stPublished}, stCancelled, reason, s.now())
	})
	if err != nil {
		return Event{}, err
	}
	s.publish(ctx, events.EventCancelled, uid, r, reason)
	return s.Get(ctx, uid, id)
}

// Feature is POST /events/:id/feature.
func (s *Service) Feature(ctx context.Context, uid, id int64) (Event, error) {
	r, err := s.owned(ctx, nil, uid, id, false)
	if err != nil {
		return Event{}, err
	}
	if r.Status != stPublished {
		return Event{}, httpx.InvalidState("Only published events can be featured.")
	}
	if r.FeaturedAt != nil {
		return s.detail(ctx, uid, r, false)
	}
	n, err := s.d.Entitlements.Int(ctx, uid, entitlements.FeaturedEvents, 0)
	if err != nil {
		return Event{}, err
	}
	if n <= 0 {
		e := httpx.Coded(403, httpx.CodePlanRequired, "Featuring events needs a premium plan.")
		e.Fields = map[string]string{"benefit": entitlements.FeaturedEvents}
		return Event{}, e
	}
	now := s.now()
	l := now.In(ist)
	from := time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, ist)
	to := from.AddDate(0, 1, 0)
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if _, err := s.r.get(ctx, tx, id, true); err != nil {
			return err
		}
		used, err := s.r.featuredCount(ctx, tx, uid, from.UTC(), to.UTC())
		if err != nil {
			return err
		}
		if used >= n {
			return httpx.LimitReached("You have used all featured events for this month.")
		}
		return s.r.setFeatured(ctx, tx, id, now)
	})
	if err != nil {
		return Event{}, err
	}
	return s.Get(ctx, uid, id)
}

// CompletePast is the event.complete_past job: published events whose end passed → completed.
func (s *Service) CompletePast(ctx context.Context) error {
	const batch, maxBatches = 500, 20
	for i := 0; i < maxBatches; i++ {
		now := s.now()
		rs, err := s.r.pastPublished(ctx, now, batch)
		if err != nil {
			return err
		}
		for _, r := range rs {
			ok, err := s.r.complete(ctx, r.ID, now)
			if err != nil {
				return err
			}
			if ok {
				s.publish(ctx, events.EventCompleted, 0, r, "")
			}
		}
		if len(rs) < batch {
			return nil
		}
	}
	return nil
}

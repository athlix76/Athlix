package registration

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/event"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/eligibility"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
)

// Service holds the registration rules.
type Service struct {
	d *app.Deps
	r *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, r: &repo{db: d.DB}} }

func (s *Service) now() time.Time { return s.d.Now().UTC() }

// ---------- validation ----------

func cleanInput(in RegisterInput) (*string, []TeamMember, error) {
	fields := map[string]string{}
	var team *string
	if in.TeamName != nil {
		if t := strings.TrimSpace(*in.TeamName); t != "" {
			if len([]rune(t)) > maxNameLen {
				fields["teamName"] = "must be at most 80 characters"
			}
			team = &t
		}
	}
	if len(in.TeamMembers) > maxTeamMembers {
		fields["teamMembers"] = "must have at most 30 members"
	}
	members := make([]TeamMember, 0, len(in.TeamMembers))
	for i, m := range in.TeamMembers {
		key := "teamMembers[" + strconv.Itoa(i) + "]"
		name := strings.TrimSpace(m.Name)
		if name == "" || len([]rune(name)) > maxNameLen {
			fields[key+".name"] = "is required (max 80 characters)"
		}
		var role *string
		if m.Role != nil {
			if r := strings.TrimSpace(*m.Role); r != "" {
				if len([]rune(r)) > maxRoleLen {
					fields[key+".role"] = "must be at most 40 characters"
				}
				role = &r
			}
		}
		members = append(members, TeamMember{Name: name, Role: role})
	}
	if len(fields) > 0 {
		return nil, nil, httpx.Validation(fields)
	}
	return team, members, nil
}

// Category eligibility reasons on 403 NOT_ELIGIBLE (docs/api.md errors). Location reasons come
// from platform/eligibility; these share the code so clients handle one error shape.
const (
	ReasonAgeGroup       = "age_group"
	ReasonGenderCategory = "gender_category"
	actionNone           = "none" // nothing the player can do in the app
)

// notEligible is 403 NOT_ELIGIBLE with fields {reason, action: "none"} and a specific message.
func notEligible(reason, msg string) *httpx.Error {
	e := httpx.NotEligible(reason, actionNone)
	e.Message = msg
	return e
}

// checkFacts applies the age band (verified birth year) and the gender category (verified
// gender). Unknown facts are allowed.
func checkFacts(e evRec, birthYear *int, gender string) error {
	if birthYear != nil {
		age := e.StartsAt.In(ist).Year() - *birthYear
		if (e.AgeMin != nil && age < *e.AgeMin) || (e.AgeMax != nil && age > *e.AgeMax) {
			return notEligible(ReasonAgeGroup, "Your age does not fit this event's age group.")
		}
	}
	switch {
	case e.GenderCategory == "male" && gender == "F", e.GenderCategory == "female" && gender == "M":
		return notEligible(ReasonGenderCategory, "This event is for "+e.GenderCategory+" players only.")
	}
	return nil
}

// check runs the location rule inside tx. Open events never load identities.
func (s *Service) check(ctx context.Context, q db.Querier, uid int64, e evRec, now time.Time) (eligibility.Result, error) {
	rule := ruleOf(e)
	if rule.Type == eligibility.TypeOpen || rule.Type == "" {
		return eligibility.Evaluate(rule, nil, now), nil
	}
	return s.d.Eligibility.Check(ctx, q, uid, rule, now)
}

func qualifiedFields(r *rec, res eligibility.Result) {
	r.QualifiedVia = res.Via
	if r.QualifiedVia == "" {
		r.QualifiedVia = eligibility.TypeOpen
	}
	r.QualifiedIdentityID, r.QualifiedDistrictID = nil, nil
	if res.IdentityID > 0 {
		id := res.IdentityID
		r.QualifiedIdentityID = &id
	}
	if res.DistrictID > 0 {
		d := res.DistrictID
		r.QualifiedDistrictID = &d
	}
}

// flagFor maps a qualifying identity's new status to the registration flag.
func flagFor(status string) string {
	switch status {
	case "expired":
		return flagExpired
	case "revoked":
		return flagRevoked
	case "rejected":
		return flagRejected
	}
	return flagChanged // withdrawn, superseded, or the identity is gone
}

// ---------- events ----------

func (s *Service) emit(ctx context.Context, typ string, actor, organiserID int64, r rec, flag string) {
	p := events.RegistrationPayload{RegistrationID: r.ID, EventID: r.EventID, UserID: r.UserID, OrganiserID: organiserID,
		Status: r.Status, PaymentStatus: r.PaymentStatus, EligibilityFlag: flag}
	if err := s.d.Events.Publish(ctx, events.New(typ, actor, p)); err != nil {
		logger.From(ctx).Error("publish event", slog.String("type", typ), slog.Int64("registrationId", r.ID), slog.String("err", err.Error()))
	}
}

func (s *Service) flush(ctx context.Context, actor int64, o *outbox) {
	for _, r := range o.created {
		s.emit(ctx, events.RegistrationCreated, actor, o.organiserID, r, "")
	}
	for _, r := range o.withdrawn {
		s.emit(ctx, events.RegistrationWithdrawn, actor, o.organiserID, r, "")
	}
	for _, r := range o.promoted {
		s.emit(ctx, events.RegistrationPromoted, actor, o.organiserID, r, "")
	}
	for _, r := range o.paid {
		s.emit(ctx, events.RegistrationPaymentMark, actor, o.organiserID, r, "")
	}
	for _, f := range o.flagged {
		s.emit(ctx, events.RegistrationEligibilityFlagged, actor, f.organiserID, f.reg, f.flag)
	}
}

// ---------- register ----------

// Register is POST /events/:id/registrations.
func (s *Service) Register(ctx context.Context, uid, eventID int64, in RegisterInput) (Registration, error) {
	team, members, err := cleanInput(in)
	if err != nil {
		return Registration{}, err
	}
	now := s.now()
	o := &outbox{}
	var out rec
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		e, err := s.r.event(ctx, tx, eventID, true)
		if err != nil {
			return err
		}
		o.organiserID = e.OrganiserID
		switch e.Status {
		case "published":
		case "cancelled", "completed":
			return httpx.InvalidState("This event is " + e.Status + ".")
		default:
			return httpx.NotFound("event")
		}
		if e.OrganiserID == uid {
			return httpx.Forbidden("You cannot register for your own event.")
		}
		blocked, err := s.d.Users.BlockedEitherWay(ctx, uid, e.OrganiserID)
		if err != nil {
			return err
		}
		if blocked {
			return httpx.NotFound("event")
		}
		if !e.Deadline.After(now) {
			return httpx.InvalidState("Registration for this event has closed.")
		}
		by, gender, err := s.r.profileFacts(ctx, tx, uid)
		if err != nil {
			return err
		}
		if err := checkFacts(e, by, gender); err != nil {
			return err
		}
		res, err := s.check(ctx, tx, uid, e, now)
		if err != nil {
			return err
		}
		if !res.Eligible {
			return httpx.NotEligible(res.Reason, res.Action)
		}
		prev, err := s.r.mine(ctx, tx, eventID, uid, true)
		if err != nil {
			return err
		}
		if prev != nil && prev.Status != stWithdrawn {
			return httpx.Conflict("You are already registered for this event.")
		}
		status := stRegistered
		if e.RegistrationsCount >= e.MaxSlots {
			if !s.d.Settings.Bool(ctx, "registration.waitlist_enabled", defWaitlistEnabled) {
				return httpx.LimitReached("This event is full.")
			}
			status = stWaitlisted
		}
		out = rec{EventID: eventID, UserID: uid, TeamName: team, TeamMembers: members, Status: status,
			PaymentStatus: "unpaid", EligibilityFlag: flagNone, CreatedAt: now}
		qualifiedFields(&out, res)
		if prev != nil {
			out.ID, out.CreatedAt = prev.ID, prev.CreatedAt
			err = s.r.reactivate(ctx, tx, out)
		} else {
			out.ID, err = s.r.insert(ctx, tx, out)
		}
		if err != nil {
			return err
		}
		if status == stRegistered {
			if err := s.r.bump(ctx, tx, eventID, 1); err != nil {
				return err
			}
		}
		o.created = append(o.created, out)
		return nil
	})
	if err != nil {
		return Registration{}, err
	}
	s.flush(ctx, uid, o)
	return Registration{Base: s.base(ctx, out)}, nil
}

// ---------- withdraw / reject / promote ----------

// promote fills one freed slot with the oldest waitlisted player who is still eligible.
// Players who no longer qualify are skipped and flagged (never cancelled).
func (s *Service) promote(ctx context.Context, tx *sql.Tx, e evRec, now time.Time, o *outbox) error {
	if !e.StartsAt.After(now) {
		return nil
	}
	ws, err := s.r.waitlisted(ctx, tx, e.ID, promoteScan)
	if err != nil {
		return err
	}
	for _, w := range ws {
		res, err := s.check(ctx, tx, w.UserID, e, now)
		if err != nil {
			return err
		}
		if res.Eligible {
			if err := s.r.qualify(ctx, tx, w.ID, stRegistered, res); err != nil {
				return err
			}
			if err := s.r.bump(ctx, tx, e.ID, 1); err != nil {
				return err
			}
			w.Status = stRegistered
			o.promoted = append(o.promoted, w)
			return nil
		}
		flag := flagChanged
		if res.Reason == eligibility.ReasonIdentityExpired {
			flag = flagExpired
		}
		if w.QualifiedIdentityID != nil {
			sts, err := s.r.identityStatuses(ctx, tx, []int64{*w.QualifiedIdentityID})
			if err != nil {
				return err
			}
			if st, ok := sts[*w.QualifiedIdentityID]; ok && st != "verified" && st != "pending" {
				flag = flagFor(st)
			}
		}
		changed, err := s.r.flag(ctx, tx, w.ID, flag, now)
		if err != nil {
			return err
		}
		if changed {
			o.flagged = append(o.flagged, flagged{reg: w, flag: flag, organiserID: e.OrganiserID})
		}
	}
	return nil
}

// Withdraw is DELETE /events/:id/registrations/me.
func (s *Service) Withdraw(ctx context.Context, uid, eventID int64) error {
	now := s.now()
	o := &outbox{}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		e, err := s.r.event(ctx, tx, eventID, true)
		if err != nil {
			return err
		}
		o.organiserID = e.OrganiserID
		r, err := s.r.mine(ctx, tx, eventID, uid, true)
		if err != nil {
			return err
		}
		if r == nil || r.Status == stWithdrawn {
			return httpx.NotFound("registration")
		}
		if r.Status == stRejected {
			return httpx.InvalidState("The organiser rejected this registration.")
		}
		hours := s.d.Settings.Int(ctx, "registration.withdraw_until_hours", defWithdrawUntilHours)
		if now.After(e.StartsAt.Add(-time.Duration(hours) * time.Hour)) {
			return httpx.InvalidState("Withdrawals close " + strconv.Itoa(hours) + " hours before the event starts.")
		}
		return s.leave(ctx, tx, e, *r, stWithdrawn, now, o)
	})
	if err != nil {
		return err
	}
	s.flush(ctx, uid, o)
	return nil
}

// leave moves r to withdrawn/rejected, frees its slot and promotes from the waitlist.
func (s *Service) leave(ctx context.Context, tx *sql.Tx, e evRec, r rec, to string, now time.Time, o *outbox) error {
	from := r.Status
	if err := s.r.setStatus(ctx, tx, r.ID, from, to); err != nil {
		return err
	}
	r.Status = to
	o.withdrawn = append(o.withdrawn, r)
	if from != stRegistered {
		return nil
	}
	if err := s.r.bump(ctx, tx, e.ID, -1); err != nil {
		return err
	}
	if e.RegistrationsCount-1 >= e.MaxSlots {
		return nil
	}
	return s.promote(ctx, tx, e, now, o)
}

// ---------- organiser ----------

// owned loads the event for its organiser (404 for anyone else).
func (s *Service) owned(ctx context.Context, q db.Querier, uid, eventID int64, forUpdate bool) (evRec, error) {
	e, err := s.r.event(ctx, q, eventID, forUpdate)
	if err != nil {
		return evRec{}, err
	}
	if e.OrganiserID != uid {
		return evRec{}, httpx.NotFound("event")
	}
	return e, nil
}

// Patch is PATCH /events/:id/registrations/:regId (event owner).
func (s *Service) Patch(ctx context.Context, uid, eventID, regID int64, in PatchInput) (ForOrganiser, error) {
	fields := map[string]string{}
	if in.PaymentStatus == nil && in.PaymentNote == nil && in.Status == nil {
		fields["paymentStatus"] = "send paymentStatus, paymentNote or status"
	}
	if in.PaymentStatus != nil && !paymentStatuses[*in.PaymentStatus] {
		fields["paymentStatus"] = "must be unpaid, paid_online, paid_offline or waived"
	}
	var note *string
	if in.PaymentNote != nil {
		if n := strings.TrimSpace(*in.PaymentNote); n != "" {
			if len([]rune(n)) > maxNoteLen {
				fields["paymentNote"] = "must be at most 200 characters"
			}
			note = &n
		}
	}
	if in.Status != nil && *in.Status != stRejected {
		fields["status"] = "can only be rejected"
	}
	if len(fields) > 0 {
		return ForOrganiser{}, httpx.Validation(fields)
	}
	now := s.now()
	o := &outbox{}
	var out rec
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		e, err := s.owned(ctx, tx, uid, eventID, true)
		if err != nil {
			return err
		}
		o.organiserID = e.OrganiserID
		r, err := s.r.onEvent(ctx, tx, eventID, regID, true)
		if err != nil {
			return err
		}
		if in.Status != nil && r.Status != stRejected {
			if r.Status == stWithdrawn {
				return httpx.InvalidState("The player has withdrawn.")
			}
			if err := s.leave(ctx, tx, e, r, stRejected, now, o); err != nil {
				return err
			}
			r.Status = stRejected
		}
		if in.PaymentStatus != nil || in.PaymentNote != nil {
			ps := r.PaymentStatus
			if in.PaymentStatus != nil {
				ps = *in.PaymentStatus
			}
			if in.PaymentNote == nil {
				note = r.PaymentNote
			}
			if err := s.r.setPayment(ctx, tx, r.ID, ps, note); err != nil {
				return err
			}
			changed := ps != r.PaymentStatus
			r.PaymentStatus, r.PaymentNote = ps, note
			if changed {
				o.paid = append(o.paid, r)
			}
		}
		out = r
		return nil
	})
	if err != nil {
		return ForOrganiser{}, err
	}
	s.flush(ctx, uid, o)
	views, err := s.organiserViews(ctx, []rec{out})
	if err != nil {
		return ForOrganiser{}, err
	}
	return views[0], nil
}

// ForEvent is GET /events/:id/registrations (event owner).
func (s *Service) ForEvent(ctx context.Context, uid, eventID int64, status string, flaggedOnly bool, pg httpx.PageParams) ([]ForOrganiser, string, error) {
	if status != "" && !statuses[status] {
		return nil, "", httpx.Validation(map[string]string{"status": "is not a valid status"})
	}
	if _, err := s.owned(ctx, nil, uid, eventID, false); err != nil {
		return nil, "", err
	}
	return s.eventPage(ctx, eventID, status, flaggedOnly, pg)
}

// AdminForEvent is GET /admin/events/:id/registrations (events.review): the organiser view
// (with qualifiedLabel) for any event; 404 when the event does not exist.
func (s *Service) AdminForEvent(ctx context.Context, eventID int64, status string, flaggedOnly bool, pg httpx.PageParams) ([]ForOrganiser, string, error) {
	if status != "" && !statuses[status] {
		return nil, "", httpx.Validation(map[string]string{"status": "is not a valid status"})
	}
	if _, err := s.r.event(ctx, nil, eventID, false); err != nil {
		return nil, "", err
	}
	return s.eventPage(ctx, eventID, status, flaggedOnly, pg)
}

func (s *Service) eventPage(ctx context.Context, eventID int64, status string, flaggedOnly bool, pg httpx.PageParams) ([]ForOrganiser, string, error) {
	rs, err := s.r.byEvent(ctx, eventID, status, flaggedOnly, pg)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rs) > pg.Limit {
		rs = rs[:pg.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rs[len(rs)-1].ID})
	}
	out, err := s.organiserViews(ctx, rs)
	return out, next, err
}

// Mine is GET /me/registrations.
func (s *Service) Mine(ctx context.Context, uid int64, status string, pg httpx.PageParams) ([]Registration, string, error) {
	if status != "" && !statuses[status] {
		return nil, "", httpx.Validation(map[string]string{"status": "is not a valid status"})
	}
	rs, err := s.r.byUser(ctx, uid, status, pg)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rs) > pg.Limit {
		rs = rs[:pg.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rs[len(rs)-1].ID})
	}
	ids, seen := []int64{}, map[int64]bool{}
	for _, r := range rs {
		if !seen[r.EventID] {
			seen[r.EventID] = true
			ids = append(ids, r.EventID)
		}
	}
	sums, err := event.Summaries(ctx, s.d, uid, ids)
	if err != nil {
		return nil, "", err
	}
	out := make([]Registration, 0, len(rs))
	for _, r := range rs {
		g := Registration{Base: s.base(ctx, r)}
		if sm, ok := sums[r.EventID]; ok {
			sm := sm
			g.Event = &sm
		}
		out = append(out, g)
	}
	return out, next, nil
}

// ---------- views ----------

func (s *Service) base(ctx context.Context, r rec) Base {
	b := Base{ID: r.ID, EventID: r.EventID, Status: r.Status, PaymentStatus: r.PaymentStatus, PaymentNote: r.PaymentNote,
		TeamName: r.TeamName, TeamMembers: r.TeamMembers, CreatedAt: r.CreatedAt, QualifiedVia: r.QualifiedVia,
		EligibilityFlag: r.EligibilityFlag, EligibilityFlaggedAt: r.EligibilityFlaggedAt}
	if b.TeamMembers == nil {
		b.TeamMembers = []TeamMember{}
	}
	if b.EligibilityFlag == "" {
		b.EligibilityFlag = flagNone
	}
	if r.QualifiedDistrictID != nil {
		if d, ok := s.d.Eligibility.Places().District(ctx, *r.QualifiedDistrictID); ok {
			b.QualifiedDistrict = &District{ID: d.ID, StateID: d.StateID, Name: d.Name}
		}
	}
	return b
}

// label is qualifiedLabel: "Native: Thanjavur (verified)", "Residence: Coimbatore (in review)", "Open".
func label(r rec, place, identityStatus string) string {
	if r.QualifiedVia == "" || r.QualifiedVia == eligibility.TypeOpen {
		return "Open"
	}
	kind := "Native"
	if r.QualifiedVia == eligibility.TypeResidence {
		kind = "Residence"
	}
	state := "no longer valid"
	switch identityStatus {
	case "verified":
		state = "verified"
	case "pending":
		state = "in review"
	case "expired":
		state = "expired"
	}
	return kind + ": " + place + " (" + state + ")"
}

func (s *Service) organiserViews(ctx context.Context, rs []rec) ([]ForOrganiser, error) {
	out := make([]ForOrganiser, 0, len(rs))
	if len(rs) == 0 {
		return out, nil
	}
	uids, idents := []int64{}, []int64{}
	seen := map[int64]bool{}
	for _, r := range rs {
		if !seen[r.UserID] {
			seen[r.UserID] = true
			uids = append(uids, r.UserID)
		}
		if r.QualifiedIdentityID != nil {
			idents = append(idents, *r.QualifiedIdentityID)
		}
	}
	users, err := s.d.Users.Summaries(ctx, uids)
	if err != nil {
		return nil, err
	}
	sts, err := s.r.identityStatuses(ctx, nil, idents)
	if err != nil {
		return nil, err
	}
	for _, r := range rs {
		v := ForOrganiser{Base: s.base(ctx, r)}
		if u, ok := users[r.UserID]; ok {
			u := u
			v.User = &u
		}
		place, st := "", ""
		if v.QualifiedDistrict != nil {
			place = v.QualifiedDistrict.Name
		}
		if r.QualifiedIdentityID != nil {
			st = sts[*r.QualifiedIdentityID]
		}
		v.QualifiedLabel = label(r, place, st)
		out = append(out, v)
	}
	return out, nil
}

// ---------- location_identity.changed ----------

// OnIdentityChanged re-evaluates registrations that qualified through an identity that is no
// longer valid. Still eligible via another identity → qualified* updated silently; otherwise
// the registration is flagged (never cancelled). Idempotent: re-delivery flags nothing new.
func (s *Service) OnIdentityChanged(ctx context.Context, e events.Event) error {
	var p events.LocationIdentityPayload
	if err := e.Decode(&p); err != nil {
		logger.From(ctx).Warn("bad location_identity.changed payload", slog.String("err", err.Error()))
		return nil
	}
	if p.IdentityID <= 0 || p.UserID <= 0 || p.Status == "verified" || p.Status == "pending" {
		return nil
	}
	if !s.d.Eligibility.Enabled(ctx) || !s.d.Settings.Bool(ctx, "location.flag_registrations_on_change", defFlagOnChange) {
		return nil
	}
	flag := flagFor(p.Status)
	now := s.now()
	var after int64
	for {
		o := &outbox{}
		var n int
		err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
			regs, evs, err := s.r.byIdentity(ctx, tx, p.UserID, p.IdentityID, after, now, batchSize)
			if err != nil {
				return err
			}
			n = len(regs)
			if n == 0 {
				return nil
			}
			ids, err := s.d.Eligibility.Load(ctx, tx, p.UserID)
			if err != nil {
				return err
			}
			for i, r := range regs {
				after = r.ID
				res := eligibility.Evaluate(ruleOf(evs[i]), ids, now)
				if res.Eligible && res.IdentityID != p.IdentityID {
					if err := s.r.qualify(ctx, tx, r.ID, r.Status, res); err != nil {
						return err
					}
					continue
				}
				changed, err := s.r.flag(ctx, tx, r.ID, flag, now)
				if err != nil {
					return err
				}
				if changed {
					o.flagged = append(o.flagged, flagged{reg: r, flag: flag, organiserID: evs[i].OrganiserID})
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		s.flush(ctx, 0, o)
		if n < batchSize {
			return nil
		}
	}
}

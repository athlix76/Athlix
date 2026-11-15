package consultation

import (
	"context"
	"database/sql"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/entitlements"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/requirements"
	"athlix/backend/internal/platform/userdir"
)

const defaultMaxOpenRequests = 5 // seed value of consultation.max_open_requests_per_player

// Service holds the consultation business rules.
type Service struct {
	d    *app.Deps
	repo *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, repo: &repo{db: d.DB}} }

func (s *Service) now() time.Time { return s.d.Now().UTC() }

func notFound() *httpx.Error { return httpx.NotFound("Consultation") }

// coachFor returns the coach if viewer may see them: an active user with the coach role
// and no block either way. Anything else is 404.
func (s *Service) coachFor(ctx context.Context, viewerID, coachID int64) (userdir.Basic, error) {
	if viewerID != coachID {
		blocked, err := s.d.Users.BlockedEitherWay(ctx, viewerID, coachID)
		if err != nil {
			return userdir.Basic{}, httpx.Internal().Wrap(err)
		}
		if blocked {
			return userdir.Basic{}, notFound()
		}
	}
	coach, err := s.d.Users.Get(ctx, coachID)
	if db.IsNoRows(err) {
		return userdir.Basic{}, notFound()
	}
	if err != nil {
		return userdir.Basic{}, httpx.Internal().Wrap(err)
	}
	if coach.Status != "active" || !coach.HasRole("coach") {
		return userdir.Basic{}, notFound()
	}
	return coach, nil
}

// planRequired reports whether this coach must hold consultation_listing to be listed.
func (s *Service) planRequired(ctx context.Context, roles []string) bool {
	return s.d.Requirements.RequiredForAny(ctx, roles, requirements.CoachProSubscription, requirements.GateOfferConsultation)
}

// live reports whether the listing is visible to others: offered and, when the
// verification requirement is on, backed by the consultation_listing benefit.
func (s *Service) live(ctx context.Context, coach userdir.Basic, l listingRow, found bool) (bool, error) {
	if !found || !l.Offered {
		return false, nil
	}
	if !s.planRequired(ctx, coach.Roles) {
		return true, nil
	}
	return s.d.Entitlements.Has(ctx, coach.ID, entitlements.ConsultationListing)
}

func (s *Service) summary(ctx context.Context, id int64) (userdir.Summary, bool, error) {
	sm, ok, err := s.d.Users.Summary(ctx, id)
	if err != nil {
		return userdir.Summary{}, false, err
	}
	return sm, ok, nil
}

// GetListing implements GET /coaches/:userId/consultation.
func (s *Service) GetListing(ctx context.Context, viewerID, coachID int64) (Listing, error) {
	coach, err := s.coachFor(ctx, viewerID, coachID)
	if err != nil {
		return Listing{}, err
	}
	l, found, err := s.repo.listing(ctx, coachID)
	if err != nil {
		return Listing{}, httpx.Internal().Wrap(err)
	}
	self := viewerID == coachID
	live, err := s.live(ctx, coach, l, found)
	if err != nil {
		return Listing{}, httpx.Internal().Wrap(err)
	}
	if !self && !live {
		return Listing{}, notFound()
	}
	sm, ok, err := s.summary(ctx, coachID)
	if err != nil {
		return Listing{}, httpx.Internal().Wrap(err)
	}
	if !ok {
		return Listing{}, notFound()
	}
	out := Listing{Coach: sm, Offered: l.Offered, Description: l.Description, FeeNote: l.FeeNote, Modes: l.Modes}
	if !self && live {
		total, toCoach, err := s.repo.pendingCounts(ctx, s.d.DB, viewerID, coachID, false)
		if err != nil {
			return Listing{}, httpx.Internal().Wrap(err)
		}
		out.CanRequest = toCoach == 0 && total < s.maxOpen(ctx)
	}
	return out, nil
}

func (s *Service) maxOpen(ctx context.Context) int {
	return s.d.Settings.Int(ctx, "consultation.max_open_requests_per_player", defaultMaxOpenRequests)
}

// MyListing returns the caller's own offer (GET /me/consultation): coaches only, and returned
// whether or not it is live, with offered=false when nothing was saved yet.
func (s *Service) MyListing(ctx context.Context, u userdir.Basic) (Listing, error) {
	if !u.HasRole("coach") {
		return Listing{}, httpx.Forbidden("Only coaches can offer consultations.")
	}
	return s.GetListing(ctx, u.ID, u.ID)
}

// PutListing implements PUT /me/consultation (coaches only).
func (s *Service) PutListing(ctx context.Context, u userdir.Basic, in ListingInput) (Listing, error) {
	if !u.HasRole("coach") {
		return Listing{}, httpx.Forbidden("Only coaches can offer consultations.")
	}
	l, _, err := s.repo.listing(ctx, u.ID)
	if err != nil {
		return Listing{}, httpx.Internal().Wrap(err)
	}
	if in.Offered != nil {
		l.Offered = *in.Offered
	}
	f := map[string]string{}
	if in.Description.Set {
		l.Description = trimmed(in.Description)
	}
	if in.FeeNote.Set {
		l.FeeNote = trimmed(in.FeeNote)
	}
	if in.Modes != nil {
		modes, ok := cleanModes(*in.Modes)
		if !ok {
			f["modes"] = "must only contain online, in_person"
		}
		l.Modes = modes
	}
	if l.Description != nil && utf8.RuneCountInString(*l.Description) > maxDescription {
		f["description"] = "must be at most 1000 characters"
	}
	if l.FeeNote != nil && utf8.RuneCountInString(*l.FeeNote) > maxFeeNote {
		f["feeNote"] = "must be at most 200 characters"
	}
	if l.Offered && len(l.Modes) == 0 && f["modes"] == "" {
		f["modes"] = "pick at least one mode"
	}
	if len(f) > 0 {
		return Listing{}, httpx.Validation(f)
	}
	if l.Offered && s.planRequired(ctx, u.Roles) {
		ok, err := s.d.Entitlements.Has(ctx, u.ID, entitlements.ConsultationListing)
		if err != nil {
			return Listing{}, httpx.Internal().Wrap(err)
		}
		if !ok {
			e := httpx.Coded(403, httpx.CodePlanRequired, "A plan with consultation listing is needed to offer consultations.")
			e.Fields = map[string]string{"benefit": entitlements.ConsultationListing}
			return Listing{}, e
		}
	}
	if err := s.repo.upsertListing(ctx, u.ID, l); err != nil {
		return Listing{}, httpx.Internal().Wrap(err)
	}
	logger.From(ctx).Info("consultation listing saved", slog.Int64("userId", u.ID), slog.Bool("offered", l.Offered))
	sm, ok, err := s.summary(ctx, u.ID)
	if err != nil {
		return Listing{}, httpx.Internal().Wrap(err)
	}
	if !ok {
		sm = userdir.Summary{ID: u.ID, DisplayName: u.DisplayName, PrimaryRole: u.PrimaryRole}
	}
	return Listing{Coach: sm, Offered: l.Offered, Description: l.Description, FeeNote: l.FeeNote, Modes: l.Modes}, nil
}

func trimmed(o Opt[string]) *string {
	if !o.Valid {
		return nil
	}
	v := strings.TrimSpace(o.V)
	if v == "" {
		return nil
	}
	return &v
}

// cleanModes de-duplicates modes in canonical order; ok is false on an unknown mode.
func cleanModes(in []string) ([]string, bool) {
	out := []string{}
	for _, m := range in {
		if !slices.Contains(Modes, m) {
			return out, false
		}
	}
	for _, m := range Modes {
		if slices.Contains(in, m) {
			out = append(out, m)
		}
	}
	return out, true
}

// CreateRequest implements POST /coaches/:userId/consultation-requests.
func (s *Service) CreateRequest(ctx context.Context, playerID, coachID int64, in RequestInput) (Request, error) {
	if playerID == coachID {
		return Request{}, httpx.Validation(map[string]string{"userId": "you cannot request a consultation from yourself"})
	}
	msg := strings.TrimSpace(in.Message)
	f := map[string]string{}
	switch n := utf8.RuneCountInString(msg); {
	case n == 0:
		f["message"] = "is required"
	case n > maxMessage:
		f["message"] = "must be at most 1000 characters"
	}
	if !slices.Contains(Modes, in.PreferredMode) {
		f["preferredMode"] = "must be online or in_person"
	}
	if len(f) > 0 {
		return Request{}, httpx.Validation(f)
	}
	coach, err := s.coachFor(ctx, playerID, coachID)
	if err != nil {
		return Request{}, err
	}
	l, found, err := s.repo.listing(ctx, coachID)
	if err != nil {
		return Request{}, httpx.Internal().Wrap(err)
	}
	live, err := s.live(ctx, coach, l, found)
	if err != nil {
		return Request{}, httpx.Internal().Wrap(err)
	}
	if !live {
		return Request{}, notFound()
	}
	if !slices.Contains(l.Modes, in.PreferredMode) {
		return Request{}, httpx.Validation(map[string]string{"preferredMode": "this coach does not offer that mode"})
	}
	limit := s.maxOpen(ctx)
	var id int64
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		total, toCoach, err := s.repo.pendingCounts(ctx, tx, playerID, coachID, true)
		if err != nil {
			return err
		}
		if toCoach > 0 {
			return httpx.Conflict("You already have a pending request with this coach.")
		}
		if total >= limit {
			return httpx.LimitReached("You have too many pending consultation requests.")
		}
		id, err = s.repo.insertRequest(ctx, tx, coachID, playerID, msg, in.PreferredMode)
		return err
	})
	if err != nil {
		return Request{}, asHTTP(err)
	}
	now := s.now()
	row := requestRow{ID: id, CoachID: coachID, PlayerID: playerID, Message: msg, PreferredMode: in.PreferredMode, Status: "pending", CreatedAt: now, UpdatedAt: now}
	s.publish(ctx, events.ConsultationRequested, playerID, row)
	return s.one(ctx, row)
}

// List implements GET /me/consultation-requests.
func (s *Service) List(ctx context.Context, userID int64, as, status string, p httpx.PageParams) ([]Request, string, error) {
	col := map[string]string{"player": colPlayer, "coach": colCoach}[as]
	f := map[string]string{}
	if col == "" {
		f["as"] = "must be coach or player"
	}
	if status != "" && !slices.Contains(Statuses, status) {
		f["status"] = "must be one of " + strings.Join(Statuses, ", ")
	}
	if len(f) > 0 {
		return nil, "", httpx.Validation(f)
	}
	rows, err := s.repo.list(ctx, col, userID, status, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", httpx.Internal().Wrap(err)
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	out, err := s.withPeople(ctx, rows)
	return out, next, err
}

// Respond implements POST /consultation-requests/:id/respond (that coach only).
func (s *Service) Respond(ctx context.Context, coachID, id int64, in RespondInput) (Request, error) {
	if in.Accept == nil {
		return Request{}, httpx.Validation(map[string]string{"accept": "is required"})
	}
	to := "declined"
	if *in.Accept {
		to = "accepted"
	}
	// in.Note is accepted for forward compatibility but has no column; it is not stored.
	row, err := s.transition(ctx, id, colCoach, coachID, "pending", to)
	if err != nil {
		return Request{}, err
	}
	s.publish(ctx, events.ConsultationResponded, coachID, row)
	return s.one(ctx, row)
}

// Cancel implements POST /consultation-requests/:id/cancel (that player, pending only).
func (s *Service) Cancel(ctx context.Context, playerID, id int64) (Request, error) {
	row, err := s.transition(ctx, id, colPlayer, playerID, "pending", "cancelled")
	if err != nil {
		return Request{}, err
	}
	return s.one(ctx, row)
}

// Complete implements POST /consultation-requests/:id/complete (that coach, accepted only).
func (s *Service) Complete(ctx context.Context, coachID, id int64) (Request, error) {
	row, err := s.transition(ctx, id, colCoach, coachID, "accepted", "completed")
	if err != nil {
		return Request{}, err
	}
	return s.one(ctx, row)
}

// transition moves a request owned by userID (via ownerCol) from one status to another.
func (s *Service) transition(ctx context.Context, id int64, ownerCol string, userID int64, from, to string) (requestRow, error) {
	var row requestRow
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		row, err = s.repo.getOwned(ctx, tx, id, ownerCol, userID)
		if db.IsNoRows(err) {
			return notFound()
		}
		if err != nil {
			return err
		}
		if row.Status != from {
			return httpx.InvalidState("This request is " + row.Status + ".")
		}
		ok, err := s.repo.setStatus(ctx, tx, id, from, to)
		if err != nil {
			return err
		}
		if !ok {
			return httpx.InvalidState("This request has changed; reload it.")
		}
		row.Status, row.UpdatedAt = to, s.now()
		return nil
	})
	if err != nil {
		return requestRow{}, asHTTP(err)
	}
	logger.From(ctx).Info("consultation request updated", slog.Int64("requestId", id), slog.String("status", to))
	return row, nil
}

func (s *Service) publish(ctx context.Context, typ string, actorID int64, r requestRow) {
	p := EventPayload{ConsultationPayload: events.ConsultationPayload{RequestID: r.ID, CoachID: r.CoachID, PlayerID: r.PlayerID, Status: r.Status}}
	if typ == events.ConsultationResponded {
		p.ResponderID = actorID // whoever acted; consumers notify the other party
	}
	e := events.New(typ, actorID, p)
	if err := s.d.Events.Publish(ctx, e); err != nil {
		logger.From(ctx).Warn("publish failed", slog.String("type", typ), slog.Int64("requestId", r.ID), slog.String("err", err.Error()))
	}
}

func (s *Service) one(ctx context.Context, r requestRow) (Request, error) {
	out, err := s.withPeople(ctx, []requestRow{r})
	if err != nil {
		return Request{}, err
	}
	return out[0], nil
}

// withPeople attaches coach and player cards. A deleted user keeps a bare {id} card.
func (s *Service) withPeople(ctx context.Context, rows []requestRow) ([]Request, error) {
	out := []Request{}
	if len(rows) == 0 {
		return out, nil
	}
	seen := map[int64]bool{}
	var ids []int64
	for _, r := range rows {
		for _, id := range []int64{r.CoachID, r.PlayerID} {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	people, err := s.d.Users.Summaries(ctx, ids)
	if err != nil {
		return nil, httpx.Internal().Wrap(err)
	}
	card := func(id int64) userdir.Summary {
		if p, ok := people[id]; ok {
			return p
		}
		return userdir.Summary{ID: id}
	}
	for _, r := range rows {
		out = append(out, Request{ID: r.ID, Coach: card(r.CoachID), Player: card(r.PlayerID), Message: r.Message,
			PreferredMode: r.PreferredMode, Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
	}
	return out, nil
}

func asHTTP(err error) error {
	if _, ok := httpx.As(err); ok {
		return err
	}
	return httpx.Internal().Wrap(err)
}

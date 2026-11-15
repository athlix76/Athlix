package community

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/entitlements"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/media"
	"athlix/backend/internal/platform/requirements"
	"athlix/backend/internal/platform/userdir"
)

var (
	errNotFound   = httpx.NotFound("community")
	errNotManager = httpx.Forbidden("Only the community owner or an admin can do this.")
	errNotOwner   = httpx.Forbidden("Only the community owner can do this.")
	errFull       = httpx.LimitReached("This community is full.")
)

// Service holds the community rules. Communities are conversations of type 'community'.
type Service struct {
	d *app.Deps
	r repo
}

// NewService builds the service (no I/O).
func NewService(d *app.Deps) *Service { return &Service{d: d} }

// PageSize is the default page size for community lists.
func (s *Service) PageSize() int { return defaultPage }

// ---- reads ----

// Get returns one community the viewer may see (private: members / invited / pending only).
func (s *Service) Get(ctx context.Context, viewer, id int64) (Community, error) {
	r, err := s.r.visible(ctx, s.d.DB, viewer, id)
	if db.IsNoRows(err) {
		return Community{}, errNotFound
	}
	if err != nil {
		return Community{}, err
	}
	out, err := s.toCommunities(ctx, []row{r})
	if err != nil {
		return Community{}, err
	}
	return out[0], nil
}

// Browse lists public, active communities (newest first).
func (s *Service) Browse(ctx context.Context, viewer int64, f ListFilter, p httpx.PageParams) ([]Community, string, error) {
	rows, err := s.r.browse(ctx, s.d.DB, viewer, f, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", err
	}
	return s.page(ctx, rows, p.Limit)
}

// Mine lists the caller's communities (active, invited or pending).
func (s *Service) Mine(ctx context.Context, viewer int64, p httpx.PageParams) ([]Community, string, error) {
	rows, err := s.r.mine(ctx, s.d.DB, viewer, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", err
	}
	return s.page(ctx, rows, p.Limit)
}

func (s *Service) page(ctx context.Context, rows []row, limit int) ([]Community, string, error) {
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	out, err := s.toCommunities(ctx, rows)
	return out, next, err
}

// toCommunities builds API objects. Callers have already applied the visibility rule, so
// signing avatar URLs here is safe.
func (s *Service) toCommunities(ctx context.Context, rows []row) ([]Community, error) {
	out := make([]Community, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	var owners, avatars []int64
	seen := map[int64]bool{}
	for _, r := range rows {
		if r.OwnerID.Valid && !seen[r.OwnerID.Int64] {
			seen[r.OwnerID.Int64] = true
			owners = append(owners, r.OwnerID.Int64)
		}
		if r.AvatarID.Valid {
			avatars = append(avatars, r.AvatarID.Int64)
		}
	}
	sums, err := s.d.Users.Summaries(ctx, owners)
	if err != nil {
		return nil, err
	}
	urls := map[int64]string{}
	if len(avatars) > 0 {
		urls = s.d.Media.ViewURLs(ctx, avatars)
	}
	for _, r := range rows {
		c := Community{
			ID: r.ID, Title: r.Title.String, Visibility: r.Visibility, JoinPolicy: r.JoinPolicy,
			MemberCount: r.MemberCount, CreatedAt: r.CreatedAt.UTC(), MyStatus: MyStatusNone,
			Description: nullStr(r.Description), City: nullStr(r.City), State: nullStr(r.State),
		}
		if r.AvatarID.Valid {
			if u, ok := urls[r.AvatarID.Int64]; ok && u != "" {
				c.AvatarURL = &u
			}
		}
		if r.SportID.Valid {
			c.Sport = &SportRef{ID: r.SportID.Int64, Code: r.SportCode.String, Name: r.SportName.String}
		}
		if r.OwnerID.Valid {
			if sm, ok := sums[r.OwnerID.Int64]; ok {
				c.Owner = &sm
			}
		}
		switch r.MyStatus.String {
		case MemberActive, MemberPending, MemberInvited, MemberBanned:
			c.MyStatus = r.MyStatus.String
		}
		if c.MyStatus == MemberActive && r.MyRole.Valid {
			role := r.MyRole.String
			c.MyRole = &role
		}
		out = append(out, c)
	}
	return out, nil
}

func nullStr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

// ---- create / update / delete ----

// ownLimit is max(chat.max_communities_created_per_user, extra_communities benefit).
func (s *Service) ownLimit(ctx context.Context, userID int64) (int, error) {
	limit := s.d.Settings.Int(ctx, "chat.max_communities_created_per_user", 10)
	extra, err := s.d.Entitlements.Int(ctx, userID, entitlements.ExtraCommunities, 0)
	if err != nil {
		return 0, err
	}
	if extra > limit {
		limit = extra
	}
	return limit, nil
}

// Create makes a community owned by the caller (one tx: conversation + owner member row).
func (s *Service) Create(ctx context.Context, u userdir.Basic, in Input) (Community, error) {
	if s.d.Requirements.RequiredForAny(ctx, u.Roles, requirements.DigiLocker, requirements.GateCreateCommunity) && u.VerificationStatus != "verified" {
		return Community{}, httpx.Coded(http.StatusForbidden, httpx.CodeVerification, "Verify your identity to create a community.")
	}
	f, errs := applyInput(fields{}, in, true)
	if len(errs) > 0 {
		return Community{}, httpx.Validation(errs)
	}
	limit, err := s.ownLimit(ctx, u.ID)
	if err != nil {
		return Community{}, err
	}
	// Serialise creates per user so two parallel requests cannot pass the limit together.
	lockKey := "community:create:" + itoa(u.ID)
	ok, err := s.d.Redis.SetNX(ctx, lockKey, "1", lockTTL).Result()
	if err != nil {
		return Community{}, err
	}
	if !ok {
		return Community{}, httpx.Conflict("Another community is being created. Try again.")
	}
	defer s.d.Redis.Del(context.WithoutCancel(ctx), lockKey)

	var id int64
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		n, err := s.r.countOwned(ctx, tx, u.ID)
		if err != nil {
			return err
		}
		if n >= limit {
			return httpx.LimitReached("You have reached the number of communities you can create.")
		}
		if err := s.checkRefs(ctx, tx, u.ID, f, fields{}); err != nil {
			return err
		}
		id, err = s.r.insert(ctx, tx, u.ID, f)
		return err
	})
	if err != nil {
		return Community{}, err
	}
	logger.From(ctx).Info("community created", slog.Int64("communityId", id), slog.Int64("userId", u.ID))
	return s.Get(ctx, u.ID, id)
}

// checkRefs validates the sport and claims a new avatar upload inside tx.
func (s *Service) checkRefs(ctx context.Context, tx *sql.Tx, userID int64, f, old fields) error {
	if f.SportID != 0 && f.SportID != old.SportID {
		ok, err := s.r.sportActive(ctx, tx, f.SportID)
		if err != nil {
			return err
		}
		if !ok {
			return httpx.Validation(map[string]string{"sportId": "is not a known sport"})
		}
	}
	if f.AvatarID != 0 && f.AvatarID != old.AvatarID {
		err := s.d.Media.Claim(ctx, tx, userID, f.AvatarID, media.PurposeCommunityAvatar)
		switch {
		case errors.Is(err, media.ErrNotFound):
			return httpx.Validation(map[string]string{"avatarUploadId": "is not a community avatar upload of yours"})
		case errors.Is(err, media.ErrNotUploaded), errors.Is(err, media.ErrMismatch):
			return httpx.Validation(map[string]string{"avatarUploadId": "upload is not complete"})
		case err != nil:
			return err
		}
	}
	return nil
}

// applyInput merges in onto cur and validates. creating applies POST defaults.
func applyInput(cur fields, in Input, creating bool) (fields, map[string]string) {
	errs := map[string]string{}
	f := cur
	if in.Title.Set && !in.Title.Null {
		f.Title = strings.TrimSpace(in.Title.V)
		if n := utf8.RuneCountInString(f.Title); n < titleMin || n > titleMax {
			errs["title"] = "must be 3 to 80 characters"
		}
	} else if creating || in.Title.Null {
		errs["title"] = "is required"
	}
	if in.Description.Set {
		f.Description = strings.TrimSpace(in.Description.V)
		if utf8.RuneCountInString(f.Description) > descriptionMax {
			errs["description"] = "must be at most 500 characters"
		}
	}
	if in.AvatarUploadID.Set {
		f.AvatarID = in.AvatarUploadID.V
		if !in.AvatarUploadID.Null && f.AvatarID <= 0 {
			errs["avatarUploadId"] = "is invalid"
		}
	}
	if in.SportID.Set {
		f.SportID = in.SportID.V
		if !in.SportID.Null && f.SportID <= 0 {
			errs["sportId"] = "is invalid"
		}
	}
	if in.City.Set {
		f.City = strings.TrimSpace(in.City.V)
		if utf8.RuneCountInString(f.City) > placeMax {
			errs["city"] = "must be at most 64 characters"
		}
	}
	if in.State.Set {
		f.State = strings.TrimSpace(in.State.V)
		if utf8.RuneCountInString(f.State) > placeMax {
			errs["state"] = "must be at most 64 characters"
		}
	}
	if in.Visibility.Set {
		f.Visibility = in.Visibility.V
		if in.Visibility.Null || (f.Visibility != VisibilityPublic && f.Visibility != VisibilityPrivate) {
			errs["visibility"] = "must be public or private"
		}
	} else if creating {
		f.Visibility = VisibilityPublic
	}
	if in.JoinPolicy.Set {
		f.JoinPolicy = in.JoinPolicy.V
		if in.JoinPolicy.Null || (f.JoinPolicy != JoinOpen && f.JoinPolicy != JoinApproval && f.JoinPolicy != JoinInvite) {
			errs["joinPolicy"] = "must be open, approval or invite"
		}
	} else if creating {
		f.JoinPolicy = JoinOpen
		if f.Visibility == VisibilityPrivate {
			f.JoinPolicy = JoinInvite
		}
	}
	if _, bad := errs["joinPolicy"]; !bad && f.Visibility == VisibilityPrivate && f.JoinPolicy == JoinOpen {
		errs["joinPolicy"] = "can't be open for a private community"
	}
	return f, errs
}

// lockFor locks a community for a write by caller. With manage set, the caller must be an
// active owner (ownerOnly) or owner/admin. Not visible → 404, visible but not allowed → 403.
func (s *Service) lockFor(ctx context.Context, tx *sql.Tx, id, caller int64, manage, ownerOnly bool) (lockRow, error) {
	l, err := s.r.lock(ctx, tx, id, caller)
	if db.IsNoRows(err) {
		return l, errNotFound
	}
	if err != nil {
		return l, err
	}
	if l.Status != StatusActive {
		return l, errNotFound
	}
	if !manage {
		return l, nil
	}
	isOwner := l.MyStatus == MemberActive && l.MyRole == RoleOwner
	isAdmin := l.MyStatus == MemberActive && l.MyRole == RoleAdmin
	if isOwner || (isAdmin && !ownerOnly) {
		return l, nil
	}
	if !l.visible() {
		return l, errNotFound
	}
	if ownerOnly {
		return l, errNotOwner
	}
	return l, errNotManager
}

// Update applies a partial CommunityInput (owner/admin).
func (s *Service) Update(ctx context.Context, caller, id int64, in Input) (Community, error) {
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if _, err := s.lockFor(ctx, tx, id, caller, true, false); err != nil {
			return err
		}
		cur, err := s.r.current(ctx, tx, id)
		if err != nil {
			return err
		}
		f, errs := applyInput(cur, in, false)
		if len(errs) > 0 {
			return httpx.Validation(errs)
		}
		if err := s.checkRefs(ctx, tx, caller, f, cur); err != nil {
			return err
		}
		return s.r.update(ctx, tx, id, f)
	})
	if err != nil {
		return Community{}, err
	}
	return s.Get(ctx, caller, id)
}

// Delete archives the community (owner only).
func (s *Service) Delete(ctx context.Context, caller, id int64) error {
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if _, err := s.lockFor(ctx, tx, id, caller, true, true); err != nil {
			return err
		}
		return s.r.setStatus(ctx, tx, id, StatusArchived)
	})
	if err == nil {
		logger.From(ctx).Info("community archived", slog.Int64("communityId", id), slog.Int64("userId", caller))
	}
	return err
}

// ---- joining ----

// minorBlocked applies minor.can_join_public_communities.
func (s *Service) minorBlocked(ctx context.Context, isMinor bool, visibility string) bool {
	return isMinor && visibility == VisibilityPublic && !s.d.Settings.Bool(ctx, "minor.can_join_public_communities", true)
}

// activate makes userID an active member, enforcing chat.group_max_members. l must be locked.
func (s *Service) activate(ctx context.Context, tx *sql.Tx, l lockRow, id, userID int64) error {
	if l.MemberCount >= s.d.Settings.Int(ctx, "chat.group_max_members", 500) {
		return errFull
	}
	if err := s.r.upsertMember(ctx, tx, id, userID, MemberActive); err != nil {
		return err
	}
	return s.r.adjustCount(ctx, tx, id, +1)
}

// Join follows the join policy: open → active, approval → pending (+ community.join_requested),
// invite → 403 unless invited (then it accepts the invite).
func (s *Service) Join(ctx context.Context, u userdir.Basic, id int64) (JoinResult, error) {
	var res JoinResult
	var requested bool
	var owner int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		l, err := s.lockFor(ctx, tx, id, u.ID, false, false)
		if err != nil {
			return err
		}
		if !l.visible() {
			return errNotFound
		}
		owner = l.OwnerID
		switch l.MyStatus {
		case MemberActive, MemberPending:
			res.MyStatus = l.MyStatus // idempotent
			return nil
		case MemberBanned:
			return httpx.Forbidden("You can't join this community.")
		}
		if s.minorBlocked(ctx, u.IsMinor, l.Visibility) {
			return httpx.Forbidden("You can't join public communities.")
		}
		if l.MyStatus == MemberInvited || l.JoinPolicy == JoinOpen {
			res.MyStatus = MemberActive
			return s.activate(ctx, tx, l, id, u.ID)
		}
		if l.JoinPolicy == JoinApproval {
			res.MyStatus = MemberPending
			requested = true
			return s.r.upsertMember(ctx, tx, id, u.ID, MemberPending)
		}
		return httpx.Forbidden("This community is invite only.")
	})
	if err != nil {
		return JoinResult{}, err
	}
	if requested {
		s.publish(ctx, events.CommunityJoinRequested, u.ID, events.CommunityJoinPayload{ConversationID: id, UserID: u.ID, OwnerID: owner})
	}
	return res, nil
}

// AcceptInvite turns the caller's invitation into an active membership.
func (s *Service) AcceptInvite(ctx context.Context, u userdir.Basic, id int64) (Community, error) {
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		l, err := s.lockFor(ctx, tx, id, u.ID, false, false)
		if err != nil {
			return err
		}
		if !l.visible() {
			return errNotFound
		}
		switch l.MyStatus {
		case MemberActive:
			return nil // already accepted
		case MemberInvited:
		default:
			return httpx.NotFound("invite")
		}
		if s.minorBlocked(ctx, u.IsMinor, l.Visibility) {
			return httpx.Forbidden("You can't join public communities.")
		}
		return s.activate(ctx, tx, l, id, u.ID)
	})
	if err != nil {
		return Community{}, err
	}
	return s.Get(ctx, u.ID, id)
}

// publish sends an event after commit; a failure is logged (ids only), the write stands.
func (s *Service) publish(ctx context.Context, typ string, actor int64, p events.CommunityJoinPayload) {
	if err := s.d.Events.Publish(ctx, events.New(typ, actor, p)); err != nil {
		logger.From(ctx).Error("community event publish failed", slog.String("type", typ), slog.Int64("communityId", p.ConversationID), slog.String("err", err.Error()))
	}
}

// emit publishes a module-local event type (EventInvited, EventOwnershipTransferred) after commit.
func (s *Service) emit(ctx context.Context, typ string, actor, id int64, p any) {
	if err := s.d.Events.Publish(ctx, events.New(typ, actor, p)); err != nil {
		logger.From(ctx).Error("community event publish failed", slog.String("type", typ), slog.Int64("communityId", id), slog.String("err", err.Error()))
	}
}

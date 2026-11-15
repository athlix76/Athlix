package user

import (
	"context"
	"crypto/rand"
	"database/sql"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/auth/otp"
	"athlix/backend/internal/modules/auth/tokens"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
)

// Service holds the user rules.
type Service struct {
	d      *app.Deps
	repo   *repo
	tokens *tokens.Store
}

func newService(d *app.Deps) *Service {
	return &Service{d: d, repo: &repo{db: d.DB}, tokens: tokens.New(d.DB, d.Now)}
}

func (s *Service) me(ctx context.Context, userID int64) (Me, error) {
	m, err := LoadMe(ctx, s.d, userID)
	if db.IsNoRows(err) {
		return Me{}, httpx.NotFound("User")
	}
	return m, err
}

// Settings returns the caller's settings.
func (s *Service) Settings(ctx context.Context, userID int64) (Settings, error) {
	return LoadSettings(ctx, s.d.DB, userID)
}

// UpdateSettings applies a partial update.
func (s *Service) UpdateSettings(ctx context.Context, userID int64, in settingsUpdate) (Settings, error) {
	if in.Privacy != nil && in.Privacy.AllowDmsFrom != nil && !slices.Contains(allowDmsValues, *in.Privacy.AllowDmsFrom) {
		return Settings{}, httpx.Validation(map[string]string{"privacy.allowDmsFrom": "must be one of everyone, followers, mutual, nobody"})
	}
	var out Settings
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := LoadSettings(ctx, tx, userID)
		if err != nil {
			return err
		}
		if p := in.NotificationPrefs; p != nil {
			setBool(&cur.NotificationPrefs.Messages, p.Messages)
			setBool(&cur.NotificationPrefs.Events, p.Events)
			setBool(&cur.NotificationPrefs.Social, p.Social)
			setBool(&cur.NotificationPrefs.Marketing, p.Marketing)
		}
		if p := in.Privacy; p != nil {
			if p.AllowDmsFrom != nil {
				cur.Privacy.AllowDmsFrom = *p.AllowDmsFrom
			}
			setBool(&cur.Privacy.ShowInSearch, p.ShowInSearch)
		}
		out = cur
		return s.repo.saveSettings(ctx, tx, userID, cur)
	})
	return out, err
}

func setBool(dst *bool, v *bool) {
	if v != nil {
		*dst = *v
	}
}

// AcceptPolicies records acceptance of active policy versions.
func (s *Service) AcceptPolicies(ctx context.Context, userID int64, ids []int64) (Me, error) {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return Me{}, httpx.Validation(map[string]string{"policyDocumentIds": "is required"})
	}
	if len(ids) > 20 {
		return Me{}, httpx.Validation(map[string]string{"policyDocumentIds": "has too many ids"})
	}
	active, err := s.repo.activePolicyIDs(ctx, ids)
	if err != nil {
		return Me{}, err
	}
	for _, id := range ids {
		if !active[id] {
			return Me{}, httpx.Validation(map[string]string{"policyDocumentIds": "contains a policy that is not active"})
		}
	}
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		for _, id := range ids {
			if err := s.repo.acceptPolicy(ctx, tx, userID, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Me{}, err
	}
	return s.me(ctx, userID)
}

func uniqueIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := []int64{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// AddRole adds a secondary role to an onboarded user (idempotent).
func (s *Service) AddRole(ctx context.Context, userID int64, role string) (Me, error) {
	if !slices.Contains(validRoles, role) {
		return Me{}, httpx.Validation(map[string]string{"role": "must be one of player, coach, organiser"})
	}
	if err := s.repo.addRole(ctx, s.d.DB, userID, role); err != nil {
		return Me{}, err
	}
	s.d.Users.Invalidate(userID)
	return s.me(ctx, userID)
}

// ChooseRole is POST /onboarding/role (only while the step is `role`).
func (s *Service) ChooseRole(ctx context.Context, userID int64, in onboardingRoleRequest) (Me, error) {
	fields := map[string]string{}
	if !slices.Contains(validRoles, in.Role) {
		fields["role"] = "must be one of player, coach, organiser"
	}
	extra := []string{}
	for _, r := range in.AdditionalRoles {
		if !slices.Contains(validRoles, r) {
			fields["additionalRoles"] = "must contain only player, coach, organiser"
			break
		}
		if r != in.Role && !slices.Contains(extra, r) {
			extra = append(extra, r)
		}
	}
	if len(fields) > 0 {
		return Me{}, httpx.Validation(fields)
	}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		step, err := s.repo.lockStep(ctx, tx, userID)
		if db.IsNoRows(err) {
			return httpx.NotFound("User")
		}
		if err != nil {
			return err
		}
		if step != StepRole {
			return httpx.InvalidState("Your role is already set.")
		}
		if err := s.repo.setPrimaryRole(ctx, tx, userID, in.Role); err != nil {
			return err
		}
		for _, r := range append([]string{in.Role}, extra...) {
			if err := s.repo.addRole(ctx, tx, userID, r); err != nil {
				return err
			}
		}
		_, err = Advance(ctx, s.d, tx, userID)
		return err
	})
	if err != nil {
		return Me{}, err
	}
	s.d.Users.Invalidate(userID)
	return s.me(ctx, userID)
}

// Delete soft-deletes the caller: PII wiped, sessions revoked, user.deleted published.
func (s *Service) Delete(ctx context.Context, userID int64) error {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	deleted := false
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		ok, err := s.repo.softDelete(ctx, tx, userID, random, s.d.Now().UTC())
		if err != nil || !ok {
			return err
		}
		deleted = true
		if err := s.repo.deleteSettings(ctx, tx, userID); err != nil {
			return err
		}
		return s.tokens.RevokeAll(ctx, tx, tokens.SubjectUser, userID)
	})
	if err != nil {
		return err
	}
	s.d.Users.Invalidate(userID)
	if deleted {
		if err := s.d.Events.Publish(ctx, events.New(events.UserDeleted, userID, events.UserPayload{UserID: userID})); err != nil {
			logger.From(ctx).Error("publish user.deleted", slog.Int64("userId", userID), slog.String("err", err.Error()))
		}
	}
	return nil
}

// ---- admin ----

var (
	userStatuses         = []string{"active", "suspended", "deleted"}
	verificationStatuses = []string{"unverified", "pending", "verified", "failed"}
)

// parseAdminFilter turns ?q&role&status&verificationStatus&isMinor into a filter.
func (s *Service) parseAdminFilter(q, role, status, verification, isMinor string) (adminUserFilter, error) {
	f := adminUserFilter{Role: role, Status: status, VerificationStatus: verification}
	fields := map[string]string{}
	if role != "" && !slices.Contains(validRoles, role) {
		fields["role"] = "must be one of player, coach, organiser"
	}
	if status != "" && !slices.Contains(userStatuses, status) {
		fields["status"] = "must be one of active, suspended, deleted"
	}
	if verification != "" && !slices.Contains(verificationStatuses, verification) {
		fields["verificationStatus"] = "must be one of unverified, pending, verified, failed"
	}
	switch strings.ToLower(isMinor) {
	case "":
	case "true", "1":
		t := true
		f.IsMinor = &t
	case "false", "0":
		v := false
		f.IsMinor = &v
	default:
		fields["isMinor"] = "must be true or false"
	}
	q = strings.TrimSpace(q)
	switch {
	case q == "":
	case strings.HasPrefix(q, "#"):
		id, err := strconv.ParseInt(q[1:], 10, 64)
		if err != nil || id <= 0 {
			fields["q"] = "user id must look like #42"
		}
		f.ID = id
	default:
		if mobile, ok := otp.NormalizeMobile(q); ok {
			f.MobileHash = s.d.Crypto.Hash("mobile", mobile)
		} else {
			if utf8.RuneCountInString(q) > 80 {
				fields["q"] = "is too long"
			}
			f.NamePrefix = q
		}
	}
	if len(fields) > 0 {
		return f, httpx.Validation(fields)
	}
	return f, nil
}

// AdminList is GET /admin/users.
func (s *Service) AdminList(ctx context.Context, f adminUserFilter) ([]AdminUserRow, string, error) {
	rows, err := s.repo.listAdmin(ctx, f)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	roles, err := s.repo.rolesFor(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	for i := range rows {
		if rs, ok := roles[rows[i].ID]; ok {
			rows[i].Roles = rs
		}
	}
	return rows, next, nil
}

// AdminDetail is GET /admin/users/:id (the caller audits the view).
func (s *Service) AdminDetail(ctx context.Context, id int64) (AdminUserDetail, error) {
	row, err := s.repo.adminRow(ctx, s.d.DB, id, false)
	if db.IsNoRows(err) {
		return AdminUserDetail{}, httpx.NotFound("User")
	}
	if err != nil {
		return AdminUserDetail{}, err
	}
	if row.Roles, err = rolesOf(ctx, s.d.DB, id); err != nil {
		return AdminUserDetail{}, err
	}
	out := AdminUserDetail{AdminUserRow: row, SuspendedReason: row.suspendedReason}
	provider, status, last4, nameEnc, gender, verifiedAt, found, err := s.repo.latestVerification(ctx, id)
	if err != nil {
		return AdminUserDetail{}, err
	}
	if found {
		v := &AdminVerification{Status: status, Provider: provider}
		if last4.Valid {
			v.AadhaarLast4 = &last4.String
		}
		if gender.Valid {
			v.Gender = &gender.String
		}
		if verifiedAt.Valid {
			t := verifiedAt.Time.UTC()
			v.VerifiedAt = &t
		}
		if len(nameEnc) > 0 {
			if name, err := s.d.Crypto.DecryptString(nameEnc); err == nil {
				v.VerifiedName = &name
			} else {
				logger.From(ctx).Error("decrypt verified name", slog.Int64("userId", id))
			}
		}
		out.Verification = v
	}
	if out.Guardian, err = s.repo.latestGuardian(ctx, id); err != nil {
		return AdminUserDetail{}, err
	}
	if out.Counts, err = s.repo.counts(ctx, id); err != nil {
		return AdminUserDetail{}, err
	}
	return out, nil
}

// changeStatus runs suspend / unsuspend in a transaction; audit is written by fn's caller via onTx.
func (s *Service) changeStatus(ctx context.Context, id int64, suspend bool, reason string, onTx func(tx *sql.Tx) error) (AdminUserRow, error) {
	var out AdminUserRow
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		row, err := s.repo.adminRow(ctx, tx, id, true)
		if db.IsNoRows(err) {
			return httpx.NotFound("User")
		}
		if err != nil {
			return err
		}
		if row.Status == "deleted" {
			return httpx.NotFound("User")
		}
		if suspend {
			if row.Status == "suspended" {
				return httpx.InvalidState("The user is already suspended.")
			}
			r := reason
			if err := s.repo.setStatus(ctx, tx, id, "suspended", &r); err != nil {
				return err
			}
			if err := s.tokens.RevokeAll(ctx, tx, tokens.SubjectUser, id); err != nil {
				return err
			}
			row.Status, row.suspendedReason = "suspended", &r
		} else {
			if row.Status != "suspended" {
				return httpx.InvalidState("The user is not suspended.")
			}
			if err := s.repo.setStatus(ctx, tx, id, "active", nil); err != nil {
				return err
			}
			row.Status, row.suspendedReason = "active", nil
		}
		if err := onTx(tx); err != nil {
			return err
		}
		out = row
		return nil
	})
	if err != nil {
		return AdminUserRow{}, err
	}
	s.d.Users.Invalidate(id)
	if out.Roles, err = rolesOf(ctx, s.d.DB, id); err != nil {
		return AdminUserRow{}, err
	}
	return out, nil
}

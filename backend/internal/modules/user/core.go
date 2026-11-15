package user

import (
	"context"
	"database/sql"
	"encoding/json"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/requirements"
)

// This file holds the helpers other agent-A modules (auth, verification, guardian, profile)
// call so the onboarding state machine and the Me shape live in one place.

// ErrNotFound is returned when the user does not exist or is deleted.
var ErrNotFound = sql.ErrNoRows

// StoreAccountToken is the stable per-user UUID passed to Play / App Store purchases
// (docs/security.md: UUID formatted from Box.Hash("store_account", userID)).
func StoreAccountToken(box *crypto.Box, userID int64) string {
	return box.StoreAccountToken(userID)
}

// LoadMe builds the Me object (3 queries, plus a signed photo URL when there is a photo).
func LoadMe(ctx context.Context, d *app.Deps, userID int64) (Me, error) {
	var m Me
	var name, role sql.NullString
	var last4 string
	var photo sql.NullInt64
	err := d.DB.QueryRowContext(ctx,
		`SELECT u.id, u.mobile_last4, u.display_name, u.primary_role, u.status, u.onboarding_step, u.verification_status, u.is_minor, u.guardian_consent_status, u.created_at, p.photo_upload_id FROM users u LEFT JOIN profiles p ON p.user_id = u.id WHERE u.id = ? AND u.status <> 'deleted'`,
		userID).Scan(&m.ID, &last4, &name, &role, &m.Status, &m.OnboardingStep, &m.VerificationStatus, &m.IsMinor, &m.GuardianConsentStatus, &m.CreatedAt, &photo)
	if err != nil {
		return Me{}, err
	}
	m.CreatedAt = m.CreatedAt.UTC()
	m.MobileMasked = crypto.MaskMobile(last4)
	if name.Valid {
		m.DisplayName = &name.String
	}
	if role.Valid {
		m.PrimaryRole = &role.String
	}
	if m.Roles, err = rolesOf(ctx, d.DB, userID); err != nil {
		return Me{}, err
	}
	if m.PendingPolicies, err = pendingPolicies(ctx, d.DB, userID, m.Roles); err != nil {
		return Me{}, err
	}
	if photo.Valid {
		if u := d.Media.ViewURL(ctx, photo.Int64); u != "" {
			m.PhotoURL = &u
		}
	}
	m.StoreAccountToken = StoreAccountToken(d.Crypto, userID)
	return m, nil
}

func rolesOf(ctx context.Context, q db.Querier, userID int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT role FROM user_roles WHERE user_id = ? ORDER BY role`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// generalPolicies are accepted by every user on the accept screen. Other types are accepted
// in their own flow: organiser_terms when the user holds the organiser role (listed here too),
// guardian_consent by the guardian in the consent flow, location_identity_notice when a
// location identity is submitted (stored as notice_version).
var generalPolicies = []any{"terms", "privacy", "community_guidelines"}

func pendingPolicies(ctx context.Context, q db.Querier, userID int64, roles []string) ([]PolicyRef, error) {
	types := append([]any{}, generalPolicies...)
	for _, r := range roles {
		if r == "organiser" {
			types = append(types, "organiser_terms")
		}
	}
	args := append(types, userID)
	rows, err := q.QueryContext(ctx,
		`SELECT d.id, d.doc_type, d.version, d.title FROM policy_documents d WHERE d.active = 1 AND d.requires_acceptance = 1 AND d.doc_type IN (`+db.Placeholders(len(types))+`) AND NOT EXISTS (SELECT 1 FROM policy_acceptances a WHERE a.user_id = ? AND a.policy_document_id = d.id) ORDER BY d.id`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PolicyRef{}
	for rows.Next() {
		var p PolicyRef
		if err := rows.Scan(&p.ID, &p.DocType, &p.Version, &p.Title); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Onboarding steps.
const (
	StepRole         = "role"
	StepVerification = "verification"
	StepGuardian     = "guardian"
	StepProfile      = "profile"
	StepDone         = "done"
)

// Advance recomputes users.onboarding_step from the user's current state and the
// verification_requirements gates, and saves it when it changed. Call it inside the
// transaction that changed the state (it locks the users row). A user who reached `done`
// never goes back. Returns the new step. Callers invalidate d.Users after commit.
func Advance(ctx context.Context, d *app.Deps, q db.Querier, userID int64) (string, error) {
	var role sql.NullString
	var step, verification, guardian string
	var isMinor bool
	var profiles int
	err := q.QueryRowContext(ctx,
		`SELECT u.primary_role, u.onboarding_step, u.verification_status, u.is_minor, u.guardian_consent_status, (SELECT COUNT(*) FROM profiles p WHERE p.user_id = u.id) FROM users u WHERE u.id = ? AND u.status <> 'deleted' FOR UPDATE`,
		userID).Scan(&role, &step, &verification, &isMinor, &guardian, &profiles)
	if err != nil {
		return "", err
	}
	if step == StepDone {
		return step, nil
	}
	roles, err := rolesOf(ctx, q, userID)
	if err != nil {
		return "", err
	}
	next := NextStep(ctx, d.Requirements, role.String, roles, verification, isMinor, guardian, profiles > 0)
	if next != step {
		if _, err := q.ExecContext(ctx, `UPDATE users SET onboarding_step = ? WHERE id = ?`, next, userID); err != nil {
			return "", err
		}
	}
	return next, nil
}

// NextStep is the pure onboarding rule (see docs/flows/signup-digilocker.md).
func NextStep(ctx context.Context, req *requirements.Checker, primaryRole string, roles []string, verification string, isMinor bool, guardian string, hasProfile bool) string {
	if primaryRole == "" {
		return StepRole
	}
	if len(roles) == 0 {
		roles = []string{primaryRole}
	}
	if req.RequiredForAny(ctx, roles, requirements.DigiLocker, requirements.GateAppAccess) && verification != "verified" {
		return StepVerification
	}
	if isMinor && req.RequiredForAny(ctx, roles, requirements.GuardianConsentIfMinor, requirements.GateAppAccess) && guardian != "granted" {
		return StepGuardian
	}
	if !hasProfile {
		return StepProfile
	}
	return StepDone
}

// SetDisplayName saves users.display_name (used by onboarding/profile and PUT /me/profile).
func SetDisplayName(ctx context.Context, q db.Querier, userID int64, name string) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET display_name = ? WHERE id = ?`, name, userID)
	return err
}

// LoadSettings returns the user's settings merged over the defaults.
func LoadSettings(ctx context.Context, q db.Querier, userID int64) (Settings, error) {
	s := DefaultSettings()
	var prefs, privacy []byte
	err := q.QueryRowContext(ctx, `SELECT notification_prefs, privacy FROM user_settings WHERE user_id = ?`, userID).Scan(&prefs, &privacy)
	if db.IsNoRows(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	_ = json.Unmarshal(prefs, &s.NotificationPrefs)
	_ = json.Unmarshal(privacy, &s.Privacy)
	if !contains(allowDmsValues, s.Privacy.AllowDmsFrom) {
		s.Privacy.AllowDmsFrom = DefaultSettings().Privacy.AllowDmsFrom
	}
	return s, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"athlix/backend/internal/platform/db"
)

type repo struct{ db *sql.DB }

func (r *repo) saveSettings(ctx context.Context, q db.Querier, userID int64, s Settings) error {
	prefs, _ := json.Marshal(s.NotificationPrefs)
	privacy, _ := json.Marshal(s.Privacy)
	_, err := q.ExecContext(ctx,
		`INSERT INTO user_settings (user_id, notification_prefs, privacy) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE notification_prefs = VALUES(notification_prefs), privacy = VALUES(privacy)`,
		userID, string(prefs), string(privacy))
	return err
}

// activePolicyIDs returns which of ids are active policy documents.
func (r *repo) activePolicyIDs(ctx context.Context, ids []int64) (map[int64]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM policy_documents WHERE active = 1 AND id IN (`+db.Placeholders(len(ids))+`)`, db.Int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (r *repo) acceptPolicy(ctx context.Context, q db.Querier, userID, docID int64) error {
	_, err := q.ExecContext(ctx, `INSERT IGNORE INTO policy_acceptances (user_id, policy_document_id) VALUES (?, ?)`, userID, docID)
	return err
}

func (r *repo) addRole(ctx context.Context, q db.Querier, userID int64, role string) error {
	_, err := q.ExecContext(ctx, `INSERT IGNORE INTO user_roles (user_id, role) VALUES (?, ?)`, userID, role)
	return err
}

// lockStep returns the onboarding step with the users row locked.
func (r *repo) lockStep(ctx context.Context, q db.Querier, userID int64) (string, error) {
	var step string
	err := q.QueryRowContext(ctx, `SELECT onboarding_step FROM users WHERE id = ? AND status <> 'deleted' FOR UPDATE`, userID).Scan(&step)
	return step, err
}

func (r *repo) setPrimaryRole(ctx context.Context, q db.Querier, userID int64, role string) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET primary_role = ? WHERE id = ?`, role, userID)
	return err
}

// softDelete wipes the user's PII columns. mobileHash is random so the number can sign up again.
func (r *repo) softDelete(ctx context.Context, q db.Querier, userID int64, mobileHash []byte, now time.Time) (bool, error) {
	res, err := q.ExecContext(ctx,
		`UPDATE users SET status = 'deleted', mobile_enc = ?, mobile_hash = ?, mobile_last4 = '0000', display_name = 'Deleted user', suspended_reason = NULL, deleted_at = ? WHERE id = ? AND status <> 'deleted'`,
		[]byte{}, mobileHash, now, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *repo) deleteSettings(ctx context.Context, q db.Querier, userID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM user_settings WHERE user_id = ?`, userID)
	return err
}

// ---- admin ----

const adminRowCols = `u.id, u.display_name, u.mobile_last4, u.primary_role, u.status, u.verification_status, u.is_minor, u.guardian_consent_status, u.onboarding_step, u.created_at, u.last_login_at, u.suspended_reason`

func scanAdminRow(sc interface{ Scan(...any) error }) (AdminUserRow, error) {
	var a AdminUserRow
	var name, role, reason sql.NullString
	var last4 string
	var lastLogin sql.NullTime
	if err := sc.Scan(&a.ID, &name, &last4, &role, &a.Status, &a.VerificationStatus, &a.IsMinor, &a.GuardianConsentStatus, &a.OnboardingStep, &a.CreatedAt, &lastLogin, &reason); err != nil {
		return a, err
	}
	a.MobileMasked = "******" + last4
	a.CreatedAt = a.CreatedAt.UTC()
	if name.Valid {
		a.DisplayName = &name.String
	}
	if role.Valid {
		a.PrimaryRole = &role.String
	}
	if lastLogin.Valid {
		t := lastLogin.Time.UTC()
		a.LastLoginAt = &t
	}
	if reason.Valid {
		a.suspendedReason = &reason.String
	}
	a.Roles = []string{}
	return a, nil
}

func (r *repo) listAdmin(ctx context.Context, f adminUserFilter) ([]AdminUserRow, error) {
	var where []string
	var args []any
	if f.ID > 0 {
		where = append(where, "u.id = ?")
		args = append(args, f.ID)
	}
	if f.MobileHash != nil {
		where = append(where, "u.mobile_hash = ?")
		args = append(args, f.MobileHash)
	}
	if f.NamePrefix != "" {
		where = append(where, "u.display_name LIKE ?")
		args = append(args, escapeLike(f.NamePrefix)+"%")
	}
	if f.Role != "" {
		where = append(where, "EXISTS (SELECT 1 FROM user_roles r WHERE r.user_id = u.id AND r.role = ?)")
		args = append(args, f.Role)
	}
	if f.Status != "" {
		where = append(where, "u.status = ?")
		args = append(args, f.Status)
	}
	if f.VerificationStatus != "" {
		where = append(where, "u.verification_status = ?")
		args = append(args, f.VerificationStatus)
	}
	if f.IsMinor != nil {
		where = append(where, "u.is_minor = ?")
		args = append(args, *f.IsMinor)
	}
	if f.BeforeID > 0 {
		where = append(where, "u.id < ?")
		args = append(args, f.BeforeID)
	}
	q := `SELECT ` + adminRowCols + ` FROM users u`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY u.id DESC LIMIT ?"
	args = append(args, f.Limit+1)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminUserRow{}
	for rows.Next() {
		a, err := scanAdminRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *repo) rolesFor(ctx context.Context, ids []int64) (map[int64][]string, error) {
	out := map[int64][]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT user_id, role FROM user_roles WHERE user_id IN (`+db.Placeholders(len(ids))+`) ORDER BY user_id, role`, db.Int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var role string
		if err := rows.Scan(&id, &role); err != nil {
			return nil, err
		}
		out[id] = append(out[id], role)
	}
	return out, rows.Err()
}

func (r *repo) adminRow(ctx context.Context, q db.Querier, id int64, forUpdate bool) (AdminUserRow, error) {
	sqlq := `SELECT ` + adminRowCols + ` FROM users u WHERE u.id = ?`
	if forUpdate {
		sqlq += " FOR UPDATE"
	}
	return scanAdminRow(q.QueryRowContext(ctx, sqlq, id))
}

func (r *repo) latestVerification(ctx context.Context, userID int64) (provider, status string, last4 sql.NullString, nameEnc []byte, gender sql.NullString, verifiedAt sql.NullTime, found bool, err error) {
	err = r.db.QueryRowContext(ctx,
		`SELECT provider, status, aadhaar_last4, name_enc, gender, verified_at FROM identity_verifications WHERE user_id = ? ORDER BY id DESC LIMIT 1`, userID).
		Scan(&provider, &status, &last4, &nameEnc, &gender, &verifiedAt)
	if db.IsNoRows(err) {
		return provider, status, last4, nil, gender, verifiedAt, false, nil
	}
	return provider, status, last4, nameEnc, gender, verifiedAt, err == nil, err
}

func (r *repo) latestGuardian(ctx context.Context, userID int64) (*AdminGuardian, error) {
	var g AdminGuardian
	var last4 string
	var granted sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT relation, guardian_mobile_last4, status, granted_at FROM guardian_consents WHERE user_id = ? ORDER BY id DESC LIMIT 1`, userID).
		Scan(&g.Relation, &last4, &g.Status, &granted)
	if db.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.MobileMasked = "******" + last4
	if granted.Valid {
		t := granted.Time.UTC()
		g.GrantedAt = &t
	}
	return &g, nil
}

func (r *repo) counts(ctx context.Context, userID int64) (AdminCounts, error) {
	var c AdminCounts
	err := r.db.QueryRowContext(ctx,
		`SELECT (SELECT COUNT(*) FROM events WHERE organiser_id = ?), (SELECT COUNT(*) FROM event_registrations WHERE user_id = ?), (SELECT COUNT(*) FROM posts WHERE author_id = ?), (SELECT COUNT(*) FROM reports WHERE target_type = 'user' AND target_id = ?)`,
		userID, userID, userID, userID).Scan(&c.EventsOrganised, &c.Registrations, &c.Posts, &c.ReportsAgainst)
	return c, err
}

func (r *repo) setStatus(ctx context.Context, q db.Querier, id int64, status string, reason *string) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET status = ?, suspended_reason = ? WHERE id = ?`, status, reason, id)
	return err
}

// escapeLike escapes LIKE wildcards in a user-supplied prefix (the pattern is still a bound arg).
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

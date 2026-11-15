package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"athlix/backend/internal/platform/db"
)

type repo struct{ db *sql.DB }

const adminCols = `a.id, a.email, a.name, a.status, a.last_login_at, r.id, r.name, r.permissions`

func scanAdmin(sc interface{ Scan(...any) error }) (AdminUser, error) {
	var a AdminUser
	var last sql.NullTime
	var perms []byte
	if err := sc.Scan(&a.ID, &a.Email, &a.Name, &a.Status, &last, &a.Role.ID, &a.Role.Name, &perms); err != nil {
		return a, err
	}
	if last.Valid {
		t := last.Time.UTC()
		a.LastLoginAt = &t
	}
	a.Permissions = parsePerms(perms)
	return a, nil
}

func parsePerms(b []byte) []string {
	out := []string{}
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func (r *repo) adminByID(ctx context.Context, q db.Querier, id int64, forUpdate bool) (AdminUser, error) {
	s := `SELECT ` + adminCols + ` FROM admin_users a JOIN admin_roles r ON r.id = a.role_id WHERE a.id = ?`
	if forUpdate {
		s += " FOR UPDATE"
	}
	return scanAdmin(q.QueryRowContext(ctx, s, id))
}

func (r *repo) listAdmins(ctx context.Context) ([]AdminUser, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+adminCols+` FROM admin_users a JOIN admin_roles r ON r.id = a.role_id ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminUser{}
	for rows.Next() {
		a, err := scanAdmin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *repo) authByEmail(ctx context.Context, q db.Querier, email string) (adminAuth, error) {
	var a adminAuth
	var locked sql.NullTime
	err := q.QueryRowContext(ctx,
		`SELECT id, password_hash, totp_secret_enc, role_id, status, failed_logins, locked_until FROM admin_users WHERE email = ? FOR UPDATE`, email).
		Scan(&a.ID, &a.PasswordHash, &a.TotpEnc, &a.RoleID, &a.Status, &a.FailedLogins, &locked)
	if locked.Valid {
		t := locked.Time.UTC()
		a.LockedUntil = &t
	}
	return a, err
}

func (r *repo) recordFailure(ctx context.Context, q db.Querier, id int64, failed int, lockedUntil *time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE admin_users SET failed_logins = ?, locked_until = ? WHERE id = ?`, failed, lockedUntil, id)
	return err
}

func (r *repo) recordLogin(ctx context.Context, q db.Querier, id int64, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE admin_users SET failed_logins = 0, locked_until = NULL, last_login_at = ? WHERE id = ?`, now, id)
	return err
}

func (r *repo) status(ctx context.Context, q db.Querier, id int64) (string, error) {
	var s string
	err := q.QueryRowContext(ctx, `SELECT status FROM admin_users WHERE id = ?`, id).Scan(&s)
	return s, err
}

func (r *repo) insertAdmin(ctx context.Context, q db.Querier, email, name, hash string, totpEnc []byte, keyVersion uint16, roleID int64) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO admin_users (email, name, password_hash, totp_secret_enc, key_version, role_id) VALUES (?, ?, ?, ?, ?, ?)`,
		email, name, hash, totpEnc, keyVersion, roleID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *repo) setCredentials(ctx context.Context, q db.Querier, id int64, hash string, totpEnc []byte, keyVersion uint16) error {
	_, err := q.ExecContext(ctx,
		`UPDATE admin_users SET password_hash = ?, totp_secret_enc = ?, key_version = ?, failed_logins = 0, locked_until = NULL WHERE id = ?`,
		hash, totpEnc, keyVersion, id)
	return err
}

func (r *repo) updateAdmin(ctx context.Context, q db.Querier, id int64, name string, roleID int64, status string) error {
	_, err := q.ExecContext(ctx, `UPDATE admin_users SET name = ?, role_id = ?, status = ? WHERE id = ?`, name, roleID, status, id)
	return err
}

func (r *repo) idByEmail(ctx context.Context, q db.Querier, email string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM admin_users WHERE email = ?`, email).Scan(&id)
	return id, err
}

// otherActiveSuperAdmins counts active admins other than id whose role grants "*".
func (r *repo) otherActiveSuperAdmins(ctx context.Context, q db.Querier, id int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM admin_users a JOIN admin_roles r ON r.id = a.role_id WHERE a.id <> ? AND a.status = 'active' AND JSON_CONTAINS(r.permissions, '"*"')`, id).Scan(&n)
	return n, err
}

// ---- roles ----

func scanRole(sc interface{ Scan(...any) error }) (Role, error) {
	var ro Role
	var desc sql.NullString
	var perms []byte
	if err := sc.Scan(&ro.ID, &ro.Name, &desc, &perms); err != nil {
		return ro, err
	}
	if desc.Valid {
		ro.Description = &desc.String
	}
	ro.Permissions = parsePerms(perms)
	return ro, nil
}

func (r *repo) roles(ctx context.Context) ([]Role, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, description, permissions FROM admin_roles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Role{}
	for rows.Next() {
		ro, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ro)
	}
	return out, rows.Err()
}

func (r *repo) role(ctx context.Context, q db.Querier, id int64, forUpdate bool) (Role, error) {
	s := `SELECT id, name, description, permissions FROM admin_roles WHERE id = ?`
	if forUpdate {
		s += " FOR UPDATE"
	}
	return scanRole(q.QueryRowContext(ctx, s, id))
}

func (r *repo) roleByName(ctx context.Context, q db.Querier, name string) (Role, error) {
	return scanRole(q.QueryRowContext(ctx, `SELECT id, name, description, permissions FROM admin_roles WHERE name = ?`, name))
}

func (r *repo) insertRole(ctx context.Context, q db.Querier, name string, desc *string, perms []string) (int64, error) {
	b, _ := json.Marshal(perms)
	res, err := q.ExecContext(ctx, `INSERT INTO admin_roles (name, description, permissions) VALUES (?, ?, ?)`, name, desc, string(b))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *repo) updateRole(ctx context.Context, q db.Querier, id int64, desc *string, perms []string) error {
	b, _ := json.Marshal(perms)
	_, err := q.ExecContext(ctx, `UPDATE admin_roles SET description = ?, permissions = ? WHERE id = ?`, desc, string(b), id)
	return err
}

// ---- audit log ----

func (r *repo) auditLog(ctx context.Context, f auditFilter) ([]AuditEntry, error) {
	var where []string
	var args []any
	if f.AdminID > 0 {
		where = append(where, "l.admin_id = ?")
		args = append(args, f.AdminID)
	}
	if f.Action != "" {
		where = append(where, "l.action = ?")
		args = append(args, f.Action)
	}
	if f.TargetType != "" {
		where = append(where, "l.target_type = ?")
		args = append(args, f.TargetType)
	}
	if f.TargetID != "" {
		where = append(where, "l.target_id = ?")
		args = append(args, f.TargetID)
	}
	if f.BeforeID > 0 {
		where = append(where, "l.id < ?")
		args = append(args, f.BeforeID)
	}
	q := `SELECT l.id, l.admin_id, COALESCE(a.name, ''), l.action, l.target_type, l.target_id, l.details, l.created_at FROM admin_audit_log l LEFT JOIN admin_users a ON a.id = l.admin_id`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY l.id DESC LIMIT ?"
	args = append(args, f.Limit+1)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var tt, tid sql.NullString
		var details []byte
		if err := rows.Scan(&e.ID, &e.Admin.ID, &e.Admin.Name, &e.Action, &tt, &tid, &details, &e.CreatedAt); err != nil {
			return nil, err
		}
		if tt.Valid {
			e.TargetType = &tt.String
		}
		if tid.Valid {
			e.TargetID = &tid.String
		}
		if len(details) > 0 {
			e.Details = json.RawMessage(details)
		} else {
			e.Details = json.RawMessage("null")
		}
		e.CreatedAt = e.CreatedAt.UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- dashboard ----

func (r *repo) dashboard(ctx context.Context, now time.Time) (Dashboard, error) {
	var d Dashboard
	week, month := now.AddDate(0, 0, -7), now.AddDate(0, 0, -30)
	err := r.db.QueryRowContext(ctx, `SELECT
 (SELECT COUNT(*) FROM users WHERE status <> 'deleted'),
 (SELECT COUNT(*) FROM users WHERE status <> 'deleted' AND verification_status = 'verified'),
 (SELECT COUNT(*) FROM users WHERE status <> 'deleted' AND is_minor = 1),
 (SELECT COUNT(*) FROM users WHERE status <> 'deleted' AND created_at >= ?),
 (SELECT COUNT(*) FROM events WHERE status = 'published'),
 (SELECT COUNT(*) FROM events WHERE status = 'pending_review'),
 (SELECT COUNT(*) FROM events WHERE status = 'published' AND starts_at > ?),
 (SELECT COUNT(*) FROM organiser_profiles WHERE status = 'submitted'),
 (SELECT COUNT(*) FROM reports WHERE status = 'open'),
 (SELECT COALESCE(SUM(total_paise), 0) FROM listing_orders WHERE status IN ('paid', 'refunded', 'partially_refunded') AND paid_at >= ?),
 (SELECT COALESCE(SUM(refund_paise), 0) FROM listing_orders WHERE refunded_at >= ?),
 (SELECT COUNT(*) FROM subscriptions WHERE status IN ('active', 'grace') AND expires_at > ?),
 (SELECT COUNT(*) FROM communities WHERE status = 'active')`,
		week, now, month, month, now).Scan(
		&d.Users.Total, &d.Users.Verified, &d.Users.Minors, &d.Users.NewLast7Days,
		&d.Events.Published, &d.Events.PendingReview, &d.Events.Upcoming,
		&d.Organisers.PendingReview, &d.Reports.Open,
		&d.Revenue.ListingPaidPaiseLast30Days, &d.Revenue.ListingRefundedPaiseLast30Days, &d.Revenue.ActiveSubscriptions,
		&d.Communities.Active)
	return d, err
}

// ---- password change ----

func (r *repo) authByID(ctx context.Context, q db.Querier, id int64) (adminAuth, error) {
	var a adminAuth
	var locked sql.NullTime
	err := q.QueryRowContext(ctx,
		`SELECT id, email, password_hash, totp_secret_enc, role_id, status, failed_logins, locked_until FROM admin_users WHERE id = ? FOR UPDATE`, id).
		Scan(&a.ID, &a.Email, &a.PasswordHash, &a.TotpEnc, &a.RoleID, &a.Status, &a.FailedLogins, &locked)
	if locked.Valid {
		t := locked.Time.UTC()
		a.LockedUntil = &t
	}
	return a, err
}

func (r *repo) setPassword(ctx context.Context, q db.Querier, id int64, hash string) error {
	_, err := q.ExecContext(ctx, `UPDATE admin_users SET password_hash = ?, failed_logins = 0, locked_until = NULL WHERE id = ?`, hash, id)
	return err
}

// ---- login events ----

func (r *repo) insertLoginEvent(ctx context.Context, q db.Querier, e loginEventRow) error {
	var adminID any
	if e.AdminID > 0 {
		adminID = e.AdminID
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO admin_login_events (admin_id, email_hash, ip, user_agent, outcome, reason) VALUES (?, ?, ?, ?, ?, ?)`,
		adminID, e.EmailHash, db.NullString(e.IP), db.NullString(e.UserAgent), e.Outcome, e.Reason)
	return err
}

func (r *repo) loginEvents(ctx context.Context, f loginEventFilter) ([]LoginEvent, error) {
	var where []string
	var args []any
	if f.AdminID > 0 {
		where = append(where, "e.admin_id = ?")
		args = append(args, f.AdminID)
	}
	if len(f.EmailHash) > 0 {
		where = append(where, "e.email_hash = ?")
		args = append(args, f.EmailHash)
	}
	if f.Outcome != "" {
		where = append(where, "e.outcome = ?")
		args = append(args, f.Outcome)
	}
	if f.BeforeID > 0 {
		where = append(where, "e.id < ?")
		args = append(args, f.BeforeID)
	}
	q := `SELECT e.id, e.admin_id, COALESCE(a.name, ''), e.outcome, e.reason, e.ip, e.user_agent, e.created_at FROM admin_login_events e LEFT JOIN admin_users a ON a.id = e.admin_id`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY e.id DESC LIMIT ?"
	args = append(args, f.Limit+1)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LoginEvent{}
	for rows.Next() {
		var e LoginEvent
		var adminID sql.NullInt64
		var name string
		var ip, ua sql.NullString
		if err := rows.Scan(&e.ID, &adminID, &name, &e.Outcome, &e.Reason, &ip, &ua, &e.CreatedAt); err != nil {
			return nil, err
		}
		if adminID.Valid {
			e.Admin = &AuditAdmin{ID: adminID.Int64, Name: name}
		}
		if ip.Valid {
			e.IP = &ip.String
		}
		if ua.Valid {
			e.UserAgent = &ua.String
		}
		e.CreatedAt = e.CreatedAt.UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// purgeLoginEvents deletes up to limit login events created before cutoff.
func (r *repo) purgeLoginEvents(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM admin_login_events WHERE created_at < ? ORDER BY id LIMIT ?`, cutoff, limit)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

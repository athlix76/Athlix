package locationidentity

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"athlix/backend/internal/platform/db"
)

// repo holds every SQL statement of the module. Bind parameters only; the only strings
// concatenated into SQL are the fixed fragments below.
type repo struct{ db *sql.DB }

const identityCols = `l.id, l.user_id, l.identity_type, l.district_id, d.name, l.state_id, s.code, s.name, s.is_ut,
	l.proof_type, l.proof_holder, l.proof_upload_id, l.aadhaar_last4, l.notice_version, l.status,
	l.rejection_code, l.rejection_reason, l.submitted_at, l.reviewed_at, l.reviewed_by, l.verified_at,
	l.expires_at, l.proof_purged_at, l.updated_at`

const identityFrom = ` FROM user_location_identities l
	JOIN districts d ON d.id = l.district_id
	JOIN states s ON s.id = l.state_id`

type scanner interface{ Scan(dest ...any) error }

func (r *identityRow) dest() []any {
	return []any{&r.ID, &r.UserID, &r.Type, &r.DistrictID, &r.DistrictName, &r.StateID, &r.StateCode, &r.StateName, &r.StateIsUT,
		&r.ProofType, &r.ProofHolder, &r.ProofUploadID, &r.AadhaarLast4, &r.NoticeVersion, &r.Status,
		&r.RejectionCode, &r.RejectionReason, &r.SubmittedAt, &r.ReviewedAt, &r.ReviewedBy, &r.VerifiedAt,
		&r.ExpiresAt, &r.ProofPurgedAt, &r.UpdatedAt}
}

func queryIdentities(ctx context.Context, q db.Querier, query string, args ...any) ([]identityRow, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []identityRow
	for rows.Next() {
		var r identityRow
		if err := rows.Scan(r.dest()...); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- geo ----

func (r *repo) states(ctx context.Context) ([]State, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, code, name, is_ut FROM states WHERE active = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []State{}
	for rows.Next() {
		var s State
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.IsUT); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *repo) adminStates(ctx context.Context) ([]AdminState, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, code, name, is_ut, lgd_code, active FROM states ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminState{}
	for rows.Next() {
		var s AdminState
		var lgd sql.NullInt64
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.IsUT, &lgd, &s.Active); err != nil {
			return nil, err
		}
		if lgd.Valid {
			v := lgd.Int64
			s.LgdCode = &v
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *repo) districts(ctx context.Context, stateID int64) ([]District, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, state_id, name FROM districts WHERE state_id = ? AND active = 1 ORDER BY name`, stateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []District{}
	for rows.Next() {
		var d District
		if err := rows.Scan(&d.ID, &d.StateID, &d.Name); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// districtWithState loads one district and its state (for POST validation and the response).
func (r *repo) districtWithState(ctx context.Context, id int64) (districtRow, error) {
	var d districtRow
	err := r.db.QueryRowContext(ctx, `SELECT d.id, d.state_id, d.name, d.active, s.code, s.name, s.is_ut
		FROM districts d JOIN states s ON s.id = d.state_id WHERE d.id = ?`, id).
		Scan(&d.ID, &d.StateID, &d.Name, &d.Active, &d.StateCode, &d.StateName, &d.StateIsUT)
	return d, err
}

// activeNotice returns the active location_identity_notice (sql.ErrNoRows if none).
func (r *repo) activeNotice(ctx context.Context) (Notice, error) {
	var n Notice
	err := r.db.QueryRowContext(ctx, `SELECT id, version, title FROM policy_documents WHERE doc_type = ? AND active = 1 ORDER BY version DESC LIMIT 1`, noticeDocType).
		Scan(&n.PolicyDocumentID, &n.Version, &n.Title)
	return n, err
}

// ---- user identities ----

func (r *repo) userIdentities(ctx context.Context, userID int64) ([]identityRow, error) {
	return queryIdentities(ctx, r.db, `SELECT `+identityCols+identityFrom+` WHERE l.user_id = ? ORDER BY l.id DESC LIMIT ?`, userID, userRowsLimit)
}

// openRow is a pending/verified row locked for a state change.
type openRow struct {
	ID         int64
	DistrictID int64
	Status     string
	VerifiedAt sql.NullTime
}

// lockOpen locks the user's pending/verified rows of one type.
func lockOpen(ctx context.Context, q db.Querier, userID int64, typ string) ([]openRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, district_id, status, verified_at FROM user_location_identities
		WHERE user_id = ? AND identity_type = ? AND status IN ('pending','verified') FOR UPDATE`, userID, typ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []openRow
	for rows.Next() {
		var o openRow
		if err := rows.Scan(&o.ID, &o.DistrictID, &o.Status, &o.VerifiedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func proofInUse(ctx context.Context, q db.Querier, uploadID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_location_identities WHERE proof_upload_id = ?`, uploadID).Scan(&n)
	return n > 0, err
}

type newIdentity struct {
	UserID        int64
	Type          string
	DistrictID    int64
	StateID       int64
	ProofType     string
	ProofHolder   string
	ProofUploadID int64
	NoticeVersion int64
	Status        string
	SubmittedAt   time.Time
	VerifiedAt    *time.Time
}

func insertIdentity(ctx context.Context, q db.Querier, n newIdentity) (int64, error) {
	var verified any
	if n.VerifiedAt != nil {
		verified = *n.VerifiedAt
	}
	res, err := q.ExecContext(ctx, `INSERT INTO user_location_identities
		(user_id, identity_type, district_id, state_id, proof_type, proof_holder, proof_upload_id, notice_version, status, submitted_at, verified_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.UserID, n.Type, n.DistrictID, n.StateID, n.ProofType, n.ProofHolder, db.NullInt64(n.ProofUploadID), db.NullInt64(n.NoticeVersion),
		n.Status, n.SubmittedAt, verified)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// acceptNotice records that the user accepted the notice document. policy_acceptances is
// owned by the user module; this is the same statement it uses (INSERT IGNORE on the
// (user_id, policy_document_id) unique key), so a repeat submit is a no-op.
func acceptNotice(ctx context.Context, q db.Querier, userID, docID int64) error {
	_, err := q.ExecContext(ctx, `INSERT IGNORE INTO policy_acceptances (user_id, policy_document_id) VALUES (?, ?)`, userID, docID)
	return err
}

// ownedForUpdate locks one row of the user (ownership in the query).
type ownedRow struct {
	ID            int64
	Type          string
	DistrictID    int64
	Status        string
	ProofUploadID sql.NullInt64
}

func lockOwned(ctx context.Context, q db.Querier, id, userID int64) (ownedRow, error) {
	var o ownedRow
	err := q.QueryRowContext(ctx, `SELECT id, identity_type, district_id, status, proof_upload_id FROM user_location_identities
		WHERE id = ? AND user_id = ? FOR UPDATE`, id, userID).Scan(&o.ID, &o.Type, &o.DistrictID, &o.Status, &o.ProofUploadID)
	return o, err
}

func setStatus(ctx context.Context, q db.Querier, id int64, from, to string) (bool, error) {
	res, err := q.ExecContext(ctx, `UPDATE user_location_identities SET status = ? WHERE id = ? AND status = ?`, to, id, from)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// markPurged records that the proof file is gone. Guarded by the upload id so a re-run is a no-op.
func (r *repo) markPurged(ctx context.Context, id, uploadID int64, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE user_location_identities SET proof_purged_at = ?, proof_upload_id = NULL WHERE id = ? AND proof_upload_id = ?`, now, id, uploadID)
	return err
}

// ---- admin ----

const adminCols = identityCols + `, u.is_minor`

const adminFrom = identityFrom + ` JOIN users u ON u.id = l.user_id`

func (r *repo) queue(ctx context.Context, f QueueFilter, afterID int64, limit int) ([]adminRow, error) {
	q := `SELECT ` + adminCols + adminFrom + ` WHERE l.status = ?`
	args := []any{f.Status}
	if f.IdentityType != "" {
		q += ` AND l.identity_type = ?`
		args = append(args, f.IdentityType)
	}
	if f.StateID > 0 {
		q += ` AND l.state_id = ?`
		args = append(args, f.StateID)
	}
	if f.DistrictID > 0 {
		q += ` AND l.district_id = ?`
		args = append(args, f.DistrictID)
	}
	asc := f.Status == statusPending // pending: oldest first
	if afterID > 0 {
		if asc {
			q += ` AND l.id > ?`
		} else {
			q += ` AND l.id < ?`
		}
		args = append(args, afterID)
	}
	if asc {
		q += ` ORDER BY l.id ASC LIMIT ?`
	} else {
		q += ` ORDER BY l.id DESC LIMIT ?`
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []adminRow
	for rows.Next() {
		var a adminRow
		if err := rows.Scan(append(a.dest(), &a.IsMinor)...); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *repo) detail(ctx context.Context, id int64) (adminRow, error) {
	var a adminRow
	err := r.db.QueryRowContext(ctx, `SELECT `+adminCols+`, l.review_note, p.district, a.name, m.content_type`+adminFrom+`
		LEFT JOIN profiles p ON p.user_id = l.user_id
		LEFT JOIN admin_users a ON a.id = l.reviewed_by
		LEFT JOIN media_uploads m ON m.id = l.proof_upload_id
		WHERE l.id = ?`, id).
		Scan(append(a.dest(), &a.IsMinor, &a.ReviewNote, &a.ProfileDistrict, &a.ReviewerName, &a.ProofContentType)...)
	return a, err
}

func (r *repo) history(ctx context.Context, userID int64, typ string, beforeID int64) ([]identityRow, error) {
	return queryIdentities(ctx, r.db, `SELECT `+identityCols+identityFrom+` WHERE l.user_id = ? AND l.identity_type = ? AND l.id < ? ORDER BY l.id DESC LIMIT ?`,
		userID, typ, beforeID, historyLimit)
}

// reviewRow is a row locked for an admin decision.
type reviewRow struct {
	ID            int64
	UserID        int64
	Type          string
	DistrictID    int64
	ProofType     string
	ProofUploadID sql.NullInt64
	Status        string
}

func lockForReview(ctx context.Context, q db.Querier, id int64) (reviewRow, error) {
	var r reviewRow
	err := q.QueryRowContext(ctx, `SELECT id, user_id, identity_type, district_id, proof_type, proof_upload_id, status
		FROM user_location_identities WHERE id = ? FOR UPDATE`, id).
		Scan(&r.ID, &r.UserID, &r.Type, &r.DistrictID, &r.ProofType, &r.ProofUploadID, &r.Status)
	return r, err
}

type verifiedRef struct {
	ID         int64
	DistrictID int64
}

func lockVerified(ctx context.Context, q db.Querier, userID int64, typ string) (*verifiedRef, error) {
	var v verifiedRef
	err := q.QueryRowContext(ctx, `SELECT id, district_id FROM user_location_identities
		WHERE user_id = ? AND identity_type = ? AND status = 'verified' FOR UPDATE`, userID, typ).Scan(&v.ID, &v.DistrictID)
	if db.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func approveRow(ctx context.Context, q db.Querier, id, adminID int64, now time.Time, expires *time.Time, last4, note string) error {
	var exp any
	if expires != nil {
		exp = *expires
	}
	_, err := q.ExecContext(ctx, `UPDATE user_location_identities SET status = 'verified', reviewed_at = ?, reviewed_by = ?, verified_at = ?,
		expires_at = ?, aadhaar_last4 = ?, review_note = ?, expiry_reminded_at = NULL WHERE id = ? AND status = 'pending'`,
		now, adminID, now, exp, db.NullString(last4), db.NullString(note), id)
	return err
}

func rejectRow(ctx context.Context, q db.Querier, id, adminID int64, now time.Time, code, reason string) error {
	_, err := q.ExecContext(ctx, `UPDATE user_location_identities SET status = 'rejected', rejection_code = ?, rejection_reason = ?,
		reviewed_at = ?, reviewed_by = ? WHERE id = ? AND status = 'pending'`,
		code, db.NullString(reason), now, adminID, id)
	return err
}

func revokeRow(ctx context.Context, q db.Querier, id int64, reason string) error {
	_, err := q.ExecContext(ctx, `UPDATE user_location_identities SET status = 'revoked', rejection_reason = ? WHERE id = ? AND status = 'verified'`,
		db.NullString(reason), id)
	return err
}

// ---- admin districts ----

func scanAliases(raw []byte) []string {
	var out []string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func aliasesArg(a []string) any {
	if len(a) == 0 {
		return nil
	}
	b, _ := json.Marshal(a)
	return string(b)
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

const districtCols = `id, state_id, name, lgd_code, aliases, active`

func scanDistrict(s scanner) (districtRow, error) {
	var d districtRow
	var aliases []byte
	if err := s.Scan(&d.ID, &d.StateID, &d.Name, &d.LgdCode, &aliases, &d.Active); err != nil {
		return d, err
	}
	d.Aliases = scanAliases(aliases)
	return d, nil
}

func (r *repo) adminDistricts(ctx context.Context, f DistrictFilter, afterID int64, limit int) ([]districtRow, error) {
	q := `SELECT ` + districtCols + ` FROM districts WHERE id > ?`
	args := []any{afterID}
	if f.StateID > 0 {
		q += ` AND state_id = ?`
		args = append(args, f.StateID)
	}
	if !f.IncludeInactive {
		q += ` AND active = 1`
	}
	if f.Q != "" {
		q += ` AND name LIKE ?`
		args = append(args, "%"+likeEscape(f.Q)+"%")
	}
	q += ` ORDER BY id LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []districtRow
	for rows.Next() {
		d, err := scanDistrict(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func stateExists(ctx context.Context, q db.Querier, id int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM states WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}

func lockDistrict(ctx context.Context, q db.Querier, id int64) (districtRow, error) {
	return scanDistrict(q.QueryRowContext(ctx, `SELECT `+districtCols+` FROM districts WHERE id = ? FOR UPDATE`, id))
}

func insertDistrict(ctx context.Context, q db.Querier, d districtRow) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO districts (state_id, name, lgd_code, aliases, active) VALUES (?, ?, ?, ?, 1)`,
		d.StateID, d.Name, nullInt(d.LgdCode), aliasesArg(d.Aliases))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func updateDistrict(ctx context.Context, q db.Querier, d districtRow) error {
	_, err := q.ExecContext(ctx, `UPDATE districts SET name = ?, lgd_code = ?, aliases = ?, active = ? WHERE id = ?`,
		d.Name, nullInt(d.LgdCode), aliasesArg(d.Aliases), d.Active, d.ID)
	return err
}

func nullInt(n sql.NullInt64) any {
	if !n.Valid {
		return nil
	}
	return n.Int64
}

// ---- events and jobs ----

func districtState(ctx context.Context, q db.Querier, districtID int64) (int64, error) {
	var stateID int64
	err := q.QueryRowContext(ctx, `SELECT state_id FROM districts WHERE id = ?`, districtID).Scan(&stateID)
	return stateID, err
}

// changedRow is the minimum needed to publish location_identity.changed.
type changedRow struct {
	ID         int64
	UserID     int64
	Type       string
	DistrictID int64
	Status     string
	ExpiresAt  sql.NullTime
}

func scanChanged(ctx context.Context, q db.Querier, query string, args ...any) ([]changedRow, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []changedRow
	for rows.Next() {
		var c changedRow
		if err := rows.Scan(&c.ID, &c.UserID, &c.Type, &c.DistrictID, &c.Status, &c.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const changedCols = `id, user_id, identity_type, district_id, status, expires_at`

func (r *repo) expiredResidences(ctx context.Context, now time.Time) ([]changedRow, error) {
	return scanChanged(ctx, r.db, `SELECT `+changedCols+` FROM user_location_identities
		WHERE status = 'verified' AND identity_type = 'residence' AND expires_at <= ? ORDER BY id LIMIT ?`, now, jobBatch)
}

func (r *repo) expiringResidences(ctx context.Context, now, until time.Time) ([]changedRow, error) {
	return scanChanged(ctx, r.db, `SELECT `+changedCols+` FROM user_location_identities
		WHERE status = 'verified' AND identity_type = 'residence' AND expires_at > ? AND expires_at <= ? AND expiry_reminded_at IS NULL
		ORDER BY id LIMIT ?`, now, until, jobBatch)
}

func (r *repo) markReminded(ctx context.Context, id int64, now time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE user_location_identities SET expiry_reminded_at = ? WHERE id = ? AND expiry_reminded_at IS NULL AND status = 'verified'`, now, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

type purgeRow struct {
	ID       int64
	UploadID int64
}

func (r *repo) purgeCandidates(ctx context.Context, cutoff time.Time) ([]purgeRow, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, proof_upload_id FROM user_location_identities
		WHERE proof_upload_id IS NOT NULL AND (status = 'withdrawn' OR (status <> 'pending' AND COALESCE(reviewed_at, updated_at) < ?))
		ORDER BY id LIMIT ?`, cutoff, jobBatch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []purgeRow
	for rows.Next() {
		var p purgeRow
		if err := rows.Scan(&p.ID, &p.UploadID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *repo) userProofUploads(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM media_uploads WHERE user_id = ? AND purpose = 'location_proof' AND status <> 'deleted' ORDER BY id LIMIT ?`, userID, jobBatch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *repo) clearUserProofs(ctx context.Context, userID int64, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE user_location_identities SET proof_purged_at = ?, proof_upload_id = NULL WHERE user_id = ? AND proof_upload_id IS NOT NULL`, now, userID)
	return err
}

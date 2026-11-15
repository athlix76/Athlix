package verification

import (
	"context"
	"database/sql"
	"time"

	"athlix/backend/internal/platform/db"
)

type repo struct{ db *sql.DB }

type userRow struct {
	VerificationStatus string
	IsMinor            bool
	PrimaryRole        sql.NullString
	DisplayName        sql.NullString
}

func (r *repo) user(ctx context.Context, q db.Querier, userID int64, forUpdate bool) (userRow, error) {
	sqlq := `SELECT verification_status, is_minor, primary_role, display_name FROM users WHERE id = ? AND status <> 'deleted'`
	if forUpdate {
		sqlq += ` FOR UPDATE`
	}
	var u userRow
	err := q.QueryRowContext(ctx, sqlq, userID).Scan(&u.VerificationStatus, &u.IsMinor, &u.PrimaryRole, &u.DisplayName)
	return u, err
}

// expireInitiated closes earlier unfinished attempts when a new one starts.
func (r *repo) expireInitiated(ctx context.Context, tx *sql.Tx, userID int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE identity_verifications SET status = 'expired' WHERE user_id = ? AND status = 'initiated'`, userID)
	return err
}

func (r *repo) insertAttempt(ctx context.Context, tx *sql.Tx, userID int64, provider, ref string) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO identity_verifications (user_id, provider, provider_ref, status) VALUES (?, ?, ?, 'initiated')`,
		userID, provider, db.NullString(ref))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// attemptForUpdate loads an attempt scoped to its owner.
func (r *repo) attemptForUpdate(ctx context.Context, tx *sql.Tx, attemptID, userID int64) (status, provider string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT status, provider FROM identity_verifications WHERE id = ? AND user_id = ? FOR UPDATE`, attemptID, userID).
		Scan(&status, &provider)
	return status, provider, err
}

// subjectOwner returns the user that already holds subjectHash (0 when none).
func (r *repo) subjectOwner(ctx context.Context, tx *sql.Tx, subjectHash []byte) (int64, error) {
	var uid int64
	err := tx.QueryRowContext(ctx, `SELECT user_id FROM identity_verifications WHERE subject_hash = ? LIMIT 1`, subjectHash).Scan(&uid)
	if db.IsNoRows(err) {
		return 0, nil
	}
	return uid, err
}

type verifiedFields struct {
	SubjectHash []byte
	Last4       sql.NullString
	NameEnc     []byte
	DOBEnc      []byte
	Gender      string
	KeyVersion  uint16
}

func (r *repo) markVerified(ctx context.Context, tx *sql.Tx, attemptID int64, f verifiedFields, now time.Time) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE identity_verifications SET status = 'verified', subject_hash = ?, aadhaar_last4 = ?, name_enc = ?, dob_enc = ?, gender = ?, key_version = ?, failure_reason = NULL, verified_at = ? WHERE id = ?`,
		f.SubjectHash, f.Last4, f.NameEnc, f.DOBEnc, f.Gender, f.KeyVersion, now, attemptID)
	return err
}

func (r *repo) markFailed(ctx context.Context, tx *sql.Tx, attemptID int64, reason string) error {
	_, err := tx.ExecContext(ctx, `UPDATE identity_verifications SET status = 'failed', failure_reason = ? WHERE id = ?`, reason, attemptID)
	return err
}

func (r *repo) setUserVerification(ctx context.Context, q db.Querier, userID int64, status string, isMinor bool) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET verification_status = ?, is_minor = ? WHERE id = ?`, status, isMinor, userID)
	return err
}

// setVerified marks the user verified. A minor whose consent is not yet tracked moves to
// guardian_consent_status 'pending' (docs/flows/minor-consent.md).
func (r *repo) setVerified(ctx context.Context, tx *sql.Tx, userID int64, isMinor bool) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE users SET verification_status = 'verified', is_minor = ?, guardian_consent_status = IF(? AND guardian_consent_status = 'not_required', 'pending', guardian_consent_status) WHERE id = ?`,
		isMinor, isMinor, userID)
	return err
}

// setPending marks the user pending unless they are already verified.
func (r *repo) setPending(ctx context.Context, tx *sql.Tx, userID int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE users SET verification_status = 'pending' WHERE id = ? AND verification_status <> 'verified'`, userID)
	return err
}

func (r *repo) setBirthYear(ctx context.Context, tx *sql.Tx, userID int64, year int) error {
	_, err := tx.ExecContext(ctx, `UPDATE profiles SET birth_year = ? WHERE user_id = ?`, year, userID)
	return err
}

type statusRow struct {
	Provider      string
	Status        string
	Last4         sql.NullString
	FailureReason sql.NullString
	VerifiedAt    sql.NullTime
}

// latest returns the verified attempt if any, else the newest attempt.
func (r *repo) latest(ctx context.Context, userID int64) (statusRow, bool, error) {
	var s statusRow
	err := r.db.QueryRowContext(ctx,
		`SELECT provider, status, aadhaar_last4, failure_reason, verified_at FROM identity_verifications WHERE user_id = ? ORDER BY status = 'verified' DESC, id DESC LIMIT 1`, userID).
		Scan(&s.Provider, &s.Status, &s.Last4, &s.FailureReason, &s.VerifiedAt)
	if db.IsNoRows(err) {
		return s, false, nil
	}
	return s, err == nil, err
}

// verifiedDOB returns the encrypted DOB of the user's verified attempt.
func (r *repo) verifiedDOB(ctx context.Context, q db.Querier, userID int64) ([]byte, error) {
	var enc []byte
	err := q.QueryRowContext(ctx, `SELECT dob_enc FROM identity_verifications WHERE user_id = ? AND status = 'verified' AND dob_enc IS NOT NULL ORDER BY id DESC LIMIT 1`, userID).Scan(&enc)
	return enc, err
}

// wipe overwrites the verification PII of a deleted user. subject_hash stays (anti-fraud).
func (r *repo) wipe(ctx context.Context, userID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE identity_verifications SET name_enc = NULL, dob_enc = NULL, gender = NULL, aadhaar_last4 = NULL, status = IF(status = 'initiated', 'expired', status) WHERE user_id = ? AND (name_enc IS NOT NULL OR dob_enc IS NOT NULL OR gender IS NOT NULL OR aadhaar_last4 IS NOT NULL OR status = 'initiated')`,
		userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type minorRow struct {
	UserID int64
	DOBEnc []byte
}

func (r *repo) minors(ctx context.Context, afterID int64, limit int) ([]minorRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT u.id, iv.dob_enc FROM users u JOIN identity_verifications iv ON iv.user_id = u.id AND iv.status = 'verified' WHERE u.is_minor = 1 AND u.status <> 'deleted' AND u.id > ? AND iv.dob_enc IS NOT NULL ORDER BY u.id LIMIT ?`,
		afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []minorRow
	for rows.Next() {
		var m minorRow
		if err := rows.Scan(&m.UserID, &m.DOBEnc); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *repo) clearMinor(ctx context.Context, tx *sql.Tx, userID int64) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE users SET is_minor = 0, guardian_consent_status = IF(guardian_consent_status = 'revoked', guardian_consent_status, 'not_required') WHERE id = ? AND is_minor = 1`, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

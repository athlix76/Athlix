package guardian

import (
	"context"
	"database/sql"

	"athlix/backend/internal/platform/db"
)

type repo struct{ db *sql.DB }

type userRow struct {
	VerificationStatus string
	IsMinor            bool
	GuardianStatus     string
	MobileHash         []byte
}

func (r *repo) user(ctx context.Context, userID int64) (userRow, error) {
	var u userRow
	err := r.db.QueryRowContext(ctx,
		`SELECT verification_status, is_minor, guardian_consent_status, mobile_hash FROM users WHERE id = ? AND status <> 'deleted'`, userID).
		Scan(&u.VerificationStatus, &u.IsMinor, &u.GuardianStatus, &u.MobileHash)
	return u, err
}

// activeConsentPolicy reports whether id is the active guardian_consent policy document.
func (r *repo) activeConsentPolicy(ctx context.Context, id int64) (bool, error) {
	var got int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM policy_documents WHERE id = ? AND doc_type = 'guardian_consent' AND active = 1`, id).Scan(&got)
	if db.IsNoRows(err) {
		return false, nil
	}
	return err == nil, err
}

type newConsent struct {
	UserID     int64
	NameEnc    []byte
	MobileEnc  []byte
	MobileHash []byte
	Last4      string
	Relation   string
	PolicyID   int64
	KeyVersion uint16
}

// insertPending expires the user's older pending consents and inserts a new one.
func (r *repo) insertPending(ctx context.Context, tx *sql.Tx, c newConsent) (int64, error) {
	if _, err := tx.ExecContext(ctx, `UPDATE guardian_consents SET status = 'expired' WHERE user_id = ? AND status = 'pending'`, c.UserID); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO guardian_consents (user_id, guardian_name_enc, guardian_mobile_enc, guardian_mobile_hash, guardian_mobile_last4, relation, status, policy_document_id, key_version) VALUES (?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
		c.UserID, c.NameEnc, c.MobileEnc, c.MobileHash, c.Last4, c.Relation, c.PolicyID, c.KeyVersion)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// markUserPending sets users.guardian_consent_status = pending unless already granted.
func (r *repo) markUserPending(ctx context.Context, tx *sql.Tx, userID int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE users SET guardian_consent_status = 'pending' WHERE id = ? AND guardian_consent_status <> 'granted'`, userID)
	return err
}

// consentStatus loads a consent scoped to its owner.
func (r *repo) consentStatus(ctx context.Context, q db.Querier, consentID, userID int64, forUpdate bool) (string, error) {
	sqlq := `SELECT status FROM guardian_consents WHERE id = ? AND user_id = ?`
	if forUpdate {
		sqlq += ` FOR UPDATE`
	}
	var st string
	err := q.QueryRowContext(ctx, sqlq, consentID, userID).Scan(&st)
	return st, err
}

func (r *repo) grant(ctx context.Context, tx *sql.Tx, consentID, userID int64, now any) error {
	if _, err := tx.ExecContext(ctx, `UPDATE guardian_consents SET status = 'granted', granted_at = ? WHERE id = ? AND user_id = ?`, now, consentID, userID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE users SET guardian_consent_status = 'granted' WHERE id = ?`, userID)
	return err
}

// wipe overwrites the guardian PII of a deleted user. Idempotent: wiped rows have an empty
// guardian_mobile_enc and are skipped.
func (r *repo) wipe(ctx context.Context, userID int64, randomHash []byte) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE guardian_consents SET guardian_name_enc = ?, guardian_mobile_enc = ?, guardian_mobile_hash = ?, guardian_mobile_last4 = '0000', status = IF(status = 'pending', 'expired', status) WHERE user_id = ? AND guardian_mobile_enc <> ''`,
		[]byte{}, []byte{}, randomHash, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// grantedConsent returns the caller's granted consent (id and encrypted guardian mobile).
func (r *repo) grantedConsent(ctx context.Context, userID int64) (int64, []byte, error) {
	var id int64
	var mobileEnc []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, guardian_mobile_enc FROM guardian_consents WHERE user_id = ? AND status = 'granted' ORDER BY id DESC LIMIT 1`, userID).
		Scan(&id, &mobileEnc)
	return id, mobileEnc, err
}

// lockGrantedConsent locks the user's granted consent; sql.ErrNoRows when there is none.
func (r *repo) lockGrantedConsent(ctx context.Context, tx *sql.Tx, userID int64) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM guardian_consents WHERE user_id = ? AND status = 'granted' ORDER BY id DESC LIMIT 1 FOR UPDATE`, userID).Scan(&id)
	return id, err
}

// revoke withdraws a granted consent. The minor goes back to the guardian onboarding step,
// so authn.RequireOnboarded blocks the app until a new consent is granted.
func (r *repo) revoke(ctx context.Context, tx *sql.Tx, consentID, userID int64, reason string, now any) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE guardian_consents SET status = 'revoked', revoked_at = ?, revoked_reason = ? WHERE id = ? AND user_id = ? AND status = 'granted'`,
		now, reason, consentID, userID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE users SET guardian_consent_status = 'revoked', onboarding_step = IF(is_minor = 1, 'guardian', onboarding_step) WHERE id = ?`, userID)
	return err
}

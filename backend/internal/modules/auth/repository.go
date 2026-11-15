package auth

import (
	"context"
	"time"

	"athlix/backend/internal/platform/db"
)

type repo struct{}

// userByMobileHash returns the user id and status for a mobile hash, row locked.
func (repo) userByMobileHash(ctx context.Context, q db.Querier, hash []byte) (int64, string, error) {
	var id int64
	var status string
	err := q.QueryRowContext(ctx, `SELECT id, status FROM users WHERE mobile_hash = ? FOR UPDATE`, hash).Scan(&id, &status)
	return id, status, err
}

func (repo) createUser(ctx context.Context, q db.Querier, mobileEnc, hash []byte, last4 string, now time.Time) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO users (mobile_enc, mobile_hash, mobile_last4, last_login_at) VALUES (?, ?, ?, ?)`, mobileEnc, hash, last4, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (repo) touchLogin(ctx context.Context, q db.Querier, id int64, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, now, id)
	return err
}

func (repo) userStatus(ctx context.Context, q db.Querier, id int64) (string, error) {
	var status string
	err := q.QueryRowContext(ctx, `SELECT status FROM users WHERE id = ?`, id).Scan(&status)
	return status, err
}

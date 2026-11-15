// Package tokens stores rotating refresh tokens (table refresh_tokens, owned by the auth
// module) for both app users and admins. Only SHA-256 hashes are stored. Every refresh
// rotates the token inside a family; presenting an already rotated token is treated as
// theft and revokes the whole family.
package tokens

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
)

// Subject types (refresh_tokens.subject_type).
const (
	SubjectUser  = "user"
	SubjectAdmin = "admin"
)

// ErrInvalid means the refresh token is unknown, expired, revoked or reused.
var ErrInvalid = errors.New("tokens: invalid refresh token")

// Issued is a newly created refresh token. Token is shown to the client once.
type Issued struct {
	ID        int64
	Token     string
	FamilyID  string
	ExpiresAt time.Time
}

// Rotated is the result of a successful refresh.
type Rotated struct {
	SubjectID int64
	DeviceID  string
	Token     Issued
}

// Store reads and writes refresh_tokens.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// New builds a Store. now may be nil (time.Now).
func New(d *sql.DB, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{db: d, now: now}
}

func (s *Store) nowUTC() time.Time { return s.now().UTC() }

// Issue creates a refresh token. familyID "" starts a new session family.
func (s *Store) Issue(ctx context.Context, q db.Querier, subjectType string, subjectID int64, familyID, deviceID string, expiresAt time.Time) (Issued, error) {
	if q == nil {
		q = s.db
	}
	raw, err := crypto.RandomToken(32)
	if err != nil {
		return Issued{}, err
	}
	if familyID == "" {
		familyID = uuid.NewString()
	}
	res, err := q.ExecContext(ctx,
		`INSERT INTO refresh_tokens (subject_type, subject_id, token_hash, family_id, device_id, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		subjectType, subjectID, crypto.SHA256(raw), familyID, db.NullString(deviceID), expiresAt.UTC())
	if err != nil {
		return Issued{}, err
	}
	id, _ := res.LastInsertId()
	return Issued{ID: id, Token: raw, FamilyID: familyID, ExpiresAt: expiresAt.UTC()}, nil
}

// Rotate exchanges raw for a new token in the same family. expiry receives the old
// token's expiry and returns the new one (sliding or fixed session). allow is called with
// the subject id before issuing and may refuse (deleted user, disabled admin).
func (s *Store) Rotate(ctx context.Context, subjectType, raw string, expiry func(old time.Time) time.Time, allow func(ctx context.Context, q db.Querier, subjectID int64) error) (Rotated, error) {
	if raw == "" {
		return Rotated{}, ErrInvalid
	}
	var out Rotated
	reused := false
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var id, subjectID int64
		var family string
		var device sql.NullString
		var expiresAt time.Time
		var revokedAt sql.NullTime
		err := tx.QueryRowContext(ctx,
			`SELECT id, subject_id, family_id, device_id, expires_at, revoked_at FROM refresh_tokens WHERE token_hash = ? AND subject_type = ? FOR UPDATE`,
			crypto.SHA256(raw), subjectType).Scan(&id, &subjectID, &family, &device, &expiresAt, &revokedAt)
		if db.IsNoRows(err) {
			return ErrInvalid
		}
		if err != nil {
			return err
		}
		now := s.nowUTC()
		if revokedAt.Valid {
			reused = true
			_, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = ? WHERE family_id = ? AND revoked_at IS NULL`, now, family)
			return err
		}
		if !expiresAt.After(now) {
			return ErrInvalid
		}
		if allow != nil {
			if err := allow(ctx, tx, subjectID); err != nil {
				return err
			}
		}
		next, err := s.Issue(ctx, tx, subjectType, subjectID, family, device.String, expiry(expiresAt))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = ?, replaced_by = ? WHERE id = ?`, now, next.ID, id); err != nil {
			return err
		}
		out = Rotated{SubjectID: subjectID, DeviceID: device.String, Token: next}
		return nil
	})
	if err != nil {
		return Rotated{}, err
	}
	if reused {
		return Rotated{}, ErrInvalid
	}
	return out, nil
}

// RevokeFamilyOf revokes the session family of raw, only if it belongs to the subject.
// Unknown tokens are ignored (logout is idempotent).
func (s *Store) RevokeFamilyOf(ctx context.Context, q db.Querier, subjectType string, subjectID int64, raw string) error {
	if q == nil {
		q = s.db
	}
	_, err := q.ExecContext(ctx,
		`UPDATE refresh_tokens t JOIN (SELECT family_id FROM refresh_tokens WHERE token_hash = ? AND subject_type = ? AND subject_id = ?) f ON f.family_id = t.family_id SET t.revoked_at = ? WHERE t.revoked_at IS NULL`,
		crypto.SHA256(raw), subjectType, subjectID, s.nowUTC())
	return err
}

// RevokeAll revokes every live session of the subject.
func (s *Store) RevokeAll(ctx context.Context, q db.Querier, subjectType string, subjectID int64) error {
	if q == nil {
		q = s.db
	}
	_, err := q.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = ? WHERE subject_type = ? AND subject_id = ? AND revoked_at IS NULL`,
		s.nowUTC(), subjectType, subjectID)
	return err
}

// Cleanup deletes up to limit tokens that expired, or were revoked, before cutoff.
func (s *Store) Cleanup(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE expires_at < ? OR revoked_at < ? LIMIT ?`, cutoff.UTC(), cutoff.UTC(), limit)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Pair is the TokenPair JSON returned by login and refresh (docs/api.md).
type Pair struct {
	AccessToken           string    `json:"accessToken"`
	AccessTokenExpiresAt  time.Time `json:"accessTokenExpiresAt"`
	RefreshToken          string    `json:"refreshToken"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
}

// NewPair builds a Pair from an access token and an issued refresh token.
func NewPair(access string, accessExp time.Time, refresh Issued) Pair {
	return Pair{AccessToken: access, AccessTokenExpiresAt: accessExp.UTC(), RefreshToken: refresh.Token, RefreshTokenExpiresAt: refresh.ExpiresAt.UTC()}
}

package tokens

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
)

var now = time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

func setup(t *testing.T) (*Store, sqlmock.Sqlmock) {
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return New(conn, func() time.Time { return now }), mock
}

var cols = []string{"id", "subject_id", "family_id", "device_id", "expires_at", "revoked_at"}

func TestIssueStoresOnlyHash(t *testing.T) {
	s, mock := setup(t)
	mock.ExpectExec(`INSERT INTO refresh_tokens`).WithArgs("user", 42, sqlmock.AnyArg(), sqlmock.AnyArg(), "dev-1", now.Add(time.Hour)).
		WillReturnResult(sqlmock.NewResult(5, 1))
	got, err := s.Issue(context.Background(), nil, SubjectUser, 42, "", "dev-1", now.Add(time.Hour))
	if err != nil || got.ID != 5 || got.FamilyID == "" || len(got.Token) < 40 {
		t.Fatal(got, err)
	}
}

func TestRotate(t *testing.T) {
	s, mock := setup(t)
	raw := "old-token"
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, subject_id, family_id, device_id, expires_at, revoked_at FROM refresh_tokens WHERE token_hash = \? AND subject_type = \? FOR UPDATE`).
		WithArgs(crypto.SHA256(raw), "user").
		WillReturnRows(sqlmock.NewRows(cols).AddRow(1, 42, "fam", "dev", now.Add(time.Hour), nil))
	mock.ExpectExec(`INSERT INTO refresh_tokens`).WithArgs("user", 42, sqlmock.AnyArg(), "fam", "dev", now.Add(48*time.Hour)).WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \?, replaced_by = \? WHERE id = \?`).WithArgs(now, 2, 1).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	allowed := int64(0)
	out, err := s.Rotate(context.Background(), SubjectUser, raw, func(time.Time) time.Time { return now.Add(48 * time.Hour) },
		func(_ context.Context, _ db.Querier, id int64) error { allowed = id; return nil })
	if err != nil || out.SubjectID != 42 || out.Token.FamilyID != "fam" || allowed != 42 {
		t.Fatal(out, err)
	}
}

func TestRotateReuseRevokesFamily(t *testing.T) {
	s, mock := setup(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM refresh_tokens WHERE token_hash`).WillReturnRows(sqlmock.NewRows(cols).AddRow(1, 42, "fam", nil, now.Add(time.Hour), now.Add(-time.Minute)))
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \? WHERE family_id = \? AND revoked_at IS NULL`).WithArgs(now, "fam").WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()
	if _, err := s.Rotate(context.Background(), SubjectUser, "x", func(o time.Time) time.Time { return o }, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestRotateExpiredUnknownAndRefused(t *testing.T) {
	s, mock := setup(t)
	ctx := context.Background()
	keep := func(o time.Time) time.Time { return o }
	if _, err := s.Rotate(ctx, SubjectUser, "", keep, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("empty token")
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM refresh_tokens`).WillReturnRows(sqlmock.NewRows(cols))
	mock.ExpectRollback()
	if _, err := s.Rotate(ctx, SubjectAdmin, "x", keep, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM refresh_tokens`).WillReturnRows(sqlmock.NewRows(cols).AddRow(1, 42, "fam", nil, now, nil))
	mock.ExpectRollback()
	if _, err := s.Rotate(ctx, SubjectUser, "x", keep, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("expired", err)
	}
	refused := errors.New("suspended")
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM refresh_tokens`).WillReturnRows(sqlmock.NewRows(cols).AddRow(1, 42, "fam", nil, now.Add(time.Hour), nil))
	mock.ExpectRollback()
	if _, err := s.Rotate(ctx, SubjectUser, "x", keep, func(context.Context, db.Querier, int64) error { return refused }); !errors.Is(err, refused) {
		t.Fatal("refused", err)
	}
}

func TestRevokeAndCleanup(t *testing.T) {
	s, mock := setup(t)
	ctx := context.Background()
	mock.ExpectExec(`UPDATE refresh_tokens t JOIN \(SELECT family_id FROM refresh_tokens WHERE token_hash = \? AND subject_type = \? AND subject_id = \?\)`).
		WithArgs(crypto.SHA256("r"), "user", 42, now).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := s.RevokeFamilyOf(ctx, nil, SubjectUser, 42, "r"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \? WHERE subject_type = \? AND subject_id = \? AND revoked_at IS NULL`).WithArgs(now, "admin", 3).WillReturnResult(sqlmock.NewResult(0, 2))
	if err := s.RevokeAll(ctx, nil, SubjectAdmin, 3); err != nil {
		t.Fatal(err)
	}
	cut := now.Add(-7 * 24 * time.Hour)
	mock.ExpectExec(`DELETE FROM refresh_tokens WHERE expires_at < \? OR revoked_at < \? LIMIT \?`).WithArgs(cut, cut, 500).WillReturnResult(sqlmock.NewResult(0, 9))
	if n, err := s.Cleanup(ctx, cut, 500); err != nil || n != 9 {
		t.Fatal(n, err)
	}
}

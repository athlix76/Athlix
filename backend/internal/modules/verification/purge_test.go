package verification

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPurgeSubjectHashes(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.subject_hash_retention_days": 90})
	job := e.m.Jobs()["verification.purge_subject_hash"]
	if job == nil {
		t.Fatal("job not registered")
	}
	cutoff := fixedNow.Add(-90 * 24 * time.Hour)
	full := sqlmock.NewRows([]string{"id"})
	ids := make([]driver.Value, 500)
	for i := range ids {
		full.AddRow(int64(i + 1))
		ids[i] = int64(i + 1)
	}
	// Only deleted users past the retention are selected; active/suspended never are.
	e.mock.ExpectQuery(`SELECT iv.id FROM identity_verifications iv JOIN users u ON u.id = iv.user_id WHERE u.status = 'deleted' AND u.deleted_at IS NOT NULL AND u.deleted_at < \? AND \(iv.subject_hash IS NOT NULL OR iv.provider_ref IS NOT NULL\) ORDER BY iv.id LIMIT \?`).
		WithArgs(cutoff, 500).WillReturnRows(full)
	e.mock.ExpectExec(`UPDATE identity_verifications iv JOIN users u ON u.id = iv.user_id SET iv.subject_hash = NULL, iv.provider_ref = NULL WHERE iv.id IN \(\?(,\?){499}\) AND u.status = 'deleted'`).
		WithArgs(ids...).WillReturnResult(sqlmock.NewResult(0, 500))
	e.mock.ExpectQuery(`SELECT iv.id FROM identity_verifications`).WithArgs(cutoff, 500).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(900)))
	e.mock.ExpectExec(`UPDATE identity_verifications iv JOIN users u`).WithArgs(int64(900)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := job(context.Background()); err != nil {
		t.Fatal(err)
	}

	// nothing to purge: no UPDATE; default retention is 180 days
	e2 := newEnv(t, nil)
	e2.mock.ExpectQuery(`SELECT iv.id FROM identity_verifications`).WithArgs(fixedNow.Add(-180*24*time.Hour), 500).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if err := e2.m.svc.PurgeSubjectHashes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e2.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

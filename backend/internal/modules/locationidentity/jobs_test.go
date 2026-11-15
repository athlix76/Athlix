package locationidentity

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/testutil"
)

func TestVerificationCompleted(t *testing.T) {
	ctx := context.Background()
	ev := func(district int64) events.Event {
		return events.New(events.VerificationCompleted, 0, events.VerificationPayload{UserID: me, Status: "verified", NativeDistrictID: district})
	}
	t.Run("no matched district does nothing", func(t *testing.T) {
		d, _ := newDeps(t, nil)
		rec := record(d, events.LocationIdentityChanged)
		if err := New(d).EventHandlers()[events.VerificationCompleted](ctx, ev(0)); err != nil || len(rec.got) != 0 {
			t.Fatalf("err %v events %+v", err, rec.got)
		}
		failed := events.New(events.VerificationCompleted, 0, events.VerificationPayload{UserID: me, Status: "failed", NativeDistrictID: thanjavur})
		if err := New(d).EventHandlers()[events.VerificationCompleted](ctx, failed); err != nil || len(rec.got) != 0 {
			t.Fatalf("failed status: err %v events %+v", err, rec.got)
		}
	})
	t.Run("creates verified native once", func(t *testing.T) {
		d, mock := newDeps(t, nil)
		rec := record(d, events.LocationIdentityChanged)
		h := New(d).EventHandlers()[events.VerificationCompleted]
		mock.ExpectBegin()
		mock.ExpectQuery(lockVerifiedRe).WithArgs(me, typeNative).WillReturnRows(sqlmock.NewRows([]string{"id", "district_id"}))
		mock.ExpectQuery(`SELECT state_id FROM districts WHERE id = \?`).WithArgs(thanjavur).WillReturnRows(sqlmock.NewRows([]string{"state_id"}).AddRow(stateTN))
		mock.ExpectExec(`INSERT INTO user_location_identities`).
			WithArgs(me, typeNative, thanjavur, stateTN, proofDigiLocker, holderSelf, nil, nil, statusVerified, now, now).
			WillReturnResult(sqlmock.NewResult(80, 1))
		mock.ExpectCommit()
		// second delivery: the verified native now exists
		mock.ExpectBegin()
		mock.ExpectQuery(lockVerifiedRe).WithArgs(me, typeNative).WillReturnRows(sqlmock.NewRows([]string{"id", "district_id"}).AddRow(80, thanjavur))
		mock.ExpectCommit()

		for i := 0; i < 2; i++ {
			if err := h(ctx, ev(thanjavur)); err != nil {
				t.Fatal(err)
			}
		}
		if len(rec.got) != 1 || rec.got[0].IdentityID != 80 || rec.got[0].Status != statusVerified || rec.got[0].IdentityType != typeNative {
			t.Fatalf("events %+v", rec.got)
		}
	})
}

func TestUserDeleted_WithdrawsAndPurges(t *testing.T) {
	d, mock := newDeps(t, nil)
	store := d.Storage.(*testutil.MemStorage)
	store.Put("location/42/99.jpg", "image/jpeg", 1)
	store.Put("location/42/98.jpg", "image/jpeg", 1)
	rec := record(d, events.LocationIdentityChanged)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM user_location_identities\s+WHERE user_id = \? AND status IN \('pending','verified'\) FOR UPDATE`).WithArgs(me).
		WillReturnRows(changedRows().AddRow(5, me, typeResidence, thanjavur, statusPending, nil).AddRow(3, me, typeNative, coimbatore, statusVerified, nil))
	mock.ExpectExec(`UPDATE user_location_identities SET status = \?`).WithArgs(statusWithdrawn, 5, statusPending).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE user_location_identities SET status = \?`).WithArgs(statusWithdrawn, 3, statusVerified).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT id FROM media_uploads WHERE user_id = \? AND purpose = 'location_proof' AND status <> 'deleted'`).WithArgs(me, jobBatch).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(98).AddRow(99))
	expectPurge(mock, 98, "location/42/98.jpg")
	expectPurge(mock, 99, "location/42/99.jpg")
	mock.ExpectExec(`UPDATE user_location_identities SET proof_purged_at = \?, proof_upload_id = NULL WHERE user_id = \? AND proof_upload_id IS NOT NULL`).
		WithArgs(now, me).WillReturnResult(sqlmock.NewResult(0, 2))

	err := New(d).EventHandlers()[events.UserDeleted](context.Background(), events.New(events.UserDeleted, me, events.UserPayload{UserID: me}))
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Objects) != 0 {
		t.Fatal("proof objects not deleted")
	}
	if len(rec.got) != 2 || rec.got[0].Status != statusWithdrawn || rec.got[1].PrevStatus != statusVerified {
		t.Fatalf("events %+v", rec.got)
	}
}

func TestJob_ExpireResidence(t *testing.T) {
	d, mock := newDeps(t, nil)
	rec := record(d, events.LocationIdentityChanged)
	mock.ExpectQuery(`WHERE status = 'verified' AND identity_type = 'residence' AND expires_at <= \? ORDER BY id LIMIT \?`).WithArgs(now, jobBatch).
		WillReturnRows(changedRows().AddRow(12, me, typeResidence, thanjavur, statusVerified, now.Add(-time.Hour)).
			AddRow(13, me+1, typeResidence, thanjavur, statusVerified, now.Add(-time.Hour)))
	mock.ExpectExec(`UPDATE user_location_identities SET status = \?`).WithArgs(statusExpired, 12, statusVerified).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE user_location_identities SET status = \?`).WithArgs(statusExpired, 13, statusVerified).WillReturnResult(sqlmock.NewResult(0, 0)) // changed meanwhile
	if err := New(d).Jobs()["location.expire_residence"](context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rec.got) != 1 || rec.got[0].IdentityID != 12 || rec.got[0].Status != statusExpired {
		t.Fatalf("events %+v", rec.got)
	}
}

func TestJob_ExpiryReminders(t *testing.T) {
	const re = `expires_at > \? AND expires_at <= \? AND expiry_reminded_at IS NULL`
	t.Run("window from config", func(t *testing.T) {
		d, mock := newDeps(t, map[string]any{"location.expiry_reminder_days": 10})
		rec := record(d, events.LocationIdentityExpiring)
		mock.ExpectQuery(re).WithArgs(now, now.Add(10*24*time.Hour), jobBatch).
			WillReturnRows(changedRows().AddRow(12, me, typeResidence, thanjavur, statusVerified, now.Add(5*24*time.Hour)))
		mock.ExpectExec(`UPDATE user_location_identities SET expiry_reminded_at = \? WHERE id = \?`).WithArgs(now, 12).WillReturnResult(sqlmock.NewResult(0, 1))
		if err := New(d).Jobs()["location.expiry_reminders"](context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(rec.got) != 1 || rec.got[0].IdentityID != 12 || rec.got[0].ExpiresAt != "2026-11-18T10:00:00Z" {
			t.Fatalf("events %+v", rec.got)
		}
	})
	t.Run("default window", func(t *testing.T) {
		d, mock := newDeps(t, nil)
		mock.ExpectQuery(re).WithArgs(now, now.Add(30*24*time.Hour), jobBatch).WillReturnRows(changedRows())
		if err := New(d).Jobs()["location.expiry_reminders"](context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("zero days disables", func(t *testing.T) {
		d, _ := newDeps(t, map[string]any{"location.expiry_reminder_days": 0})
		if err := New(d).Jobs()["location.expiry_reminders"](context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestJob_PurgeProofs(t *testing.T) {
	const re = `SELECT id, proof_upload_id FROM user_location_identities\s+WHERE proof_upload_id IS NOT NULL AND \(status = 'withdrawn' OR \(status <> 'pending' AND COALESCE\(reviewed_at, updated_at\) < \?\)\)`
	t.Run("retention from config", func(t *testing.T) {
		d, mock := newDeps(t, map[string]any{"location.proof_retention_days": 7})
		d.Storage.(*testutil.MemStorage).Put("location/42/99.jpg", "image/jpeg", 1)
		mock.ExpectQuery(re).WithArgs(now.Add(-7*24*time.Hour), jobBatch).WillReturnRows(sqlmock.NewRows([]string{"id", "proof_upload_id"}).AddRow(12, 99))
		expectPurge(mock, 99, "location/42/99.jpg")
		expectMarkPurged(mock, 12, 99)
		if err := New(d).Jobs()["location.purge_proofs"](context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(d.Storage.(*testutil.MemStorage).Objects) != 0 {
			t.Fatal("object not deleted")
		}
	})
	t.Run("default retention", func(t *testing.T) {
		d, mock := newDeps(t, nil)
		mock.ExpectQuery(re).WithArgs(now.Add(-30*24*time.Hour), jobBatch).WillReturnRows(sqlmock.NewRows([]string{"id", "proof_upload_id"}))
		if err := New(d).Jobs()["location.purge_proofs"](context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

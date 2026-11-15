package media

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/storage"
	"athlix/backend/internal/testutil"
)

const uid = int64(42)

var fixedNow = time.Date(2026, 6, 15, 6, 0, 0, 0, time.UTC)

type envelope struct {
	Data struct {
		UploadID  int64             `json:"uploadId"`
		UploadURL string            `json:"uploadUrl"`
		Method    string            `json:"method"`
		Headers   map[string]string `json:"headers"`
		Status    string            `json:"status"`
	} `json:"data"`
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func decode(t *testing.T, b []byte) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return e
}

func TestCreateUploadBackendBuildsKey(t *testing.T) {
	d, mock := testutil.Deps(t, map[string]any{"media.image_max_mb": 2})
	r, routes := testutil.UserEngine(testutil.VerifiedUser(uid))
	New(d).Routes(routes)

	var key string
	mock.ExpectExec(`INSERT INTO media_uploads \(user_id, purpose, s3_key, content_type, size_bytes, status\)`).
		WithArgs(uid, "profile_photo", sqlmock.AnyArg(), "image/png", int64(1000)).
		WillReturnResult(sqlmock.NewResult(77, 1))
	w := testutil.Request(r, http.MethodPost, "/v1/media/uploads", map[string]any{"purpose": "profile_photo", "contentType": "image/png", "sizeBytes": 1000, "key": "../../etc/passwd"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	e := decode(t, w.Body.Bytes())
	if e.Data.UploadID != 77 || e.Data.Method != "PUT" || e.Data.Headers["Content-Type"] != "image/png" {
		t.Fatalf("ticket %+v", e.Data)
	}
	key = strings.TrimPrefix(e.Data.UploadURL, "https://storage.test/put/")
	if !strings.HasPrefix(key, "profile_photo/42/") || strings.Contains(key, "..") || !strings.HasSuffix(key, ".png") {
		t.Fatalf("key %q not built by the backend", key)
	}
}

func TestCreateUploadValidation(t *testing.T) {
	d, _ := testutil.Deps(t, map[string]any{"media.image_max_mb": 1, "media.allowed_image_types": []string{"image/jpeg"}})
	r, routes := testutil.UserEngine(testutil.VerifiedUser(uid))
	New(d).Routes(routes)
	cases := []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"purpose": "secret_docs", "contentType": "image/jpeg", "sizeBytes": 10}, "purpose"},
		{map[string]any{"purpose": "profile_photo", "contentType": "image/png", "sizeBytes": 10}, "contentType"}, // not in the configured list
		{map[string]any{"purpose": "profile_photo", "contentType": "image/jpeg", "sizeBytes": 2 << 20}, "sizeBytes"},
		{map[string]any{"purpose": "profile_photo", "contentType": "image/jpeg", "sizeBytes": 0}, "sizeBytes"},
	}
	for _, tc := range cases {
		w := testutil.Request(r, http.MethodPost, "/v1/media/uploads", tc.body, nil)
		if w.Code != http.StatusUnprocessableEntity || decode(t, w.Body.Bytes()).Error.Fields[tc.field] == "" {
			t.Errorf("%v: %d %s", tc.body, w.Code, w.Body)
		}
	}
}

func expectUpload(mock sqlmock.Sqlmock, id, owner int64, key, status string) {
	rows := sqlmock.NewRows([]string{"id", "user_id", "purpose", "s3_key", "content_type", "size_bytes", "status"})
	if key != "" {
		rows.AddRow(id, owner, "profile_photo", key, "image/png", 1000, status)
	}
	mock.ExpectQuery(`SELECT id, user_id, purpose, s3_key, content_type, size_bytes, status FROM media_uploads WHERE id = \? AND user_id = \?`).
		WithArgs(id, owner).WillReturnRows(rows)
}

func TestConfirm(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	mem := d.Storage.(*testutil.MemStorage)
	r, routes := testutil.UserEngine(testutil.VerifiedUser(uid))
	New(d).Routes(routes)

	// Not uploaded yet → 409.
	expectUpload(mock, 7, uid, "profile_photo/42/a.png", "pending")
	if w := testutil.Request(r, http.MethodPost, "/v1/media/uploads/7/confirm", map[string]any{}, nil); w.Code != http.StatusConflict {
		t.Fatalf("not uploaded: %d %s", w.Code, w.Body)
	}

	// Uploaded with the declared size and type → confirmed.
	mem.Put("profile_photo/42/a.png", "image/png", 1000)
	expectUpload(mock, 7, uid, "profile_photo/42/a.png", "pending")
	mock.ExpectExec(`UPDATE media_uploads SET status = 'confirmed'`).WithArgs(int64(7), uid).WillReturnResult(sqlmock.NewResult(0, 1))
	w := testutil.Request(r, http.MethodPost, "/v1/media/uploads/7/confirm", map[string]any{}, nil)
	if w.Code != http.StatusOK || decode(t, w.Body.Bytes()).Data.Status != "confirmed" {
		t.Fatalf("confirm: %d %s", w.Code, w.Body)
	}

	// Already attached: still reported as confirmed, no write.
	expectUpload(mock, 7, uid, "profile_photo/42/a.png", "attached")
	w = testutil.Request(r, http.MethodPost, "/v1/media/uploads/7/confirm", map[string]any{}, nil)
	if w.Code != http.StatusOK || decode(t, w.Body.Bytes()).Data.Status != "confirmed" {
		t.Fatalf("attached: %d %s", w.Code, w.Body)
	}

	// Size mismatch → 422, object and row removed.
	mem.Put("profile_photo/42/b.png", "image/png", 999999)
	expectUpload(mock, 8, uid, "profile_photo/42/b.png", "pending")
	mock.ExpectExec(`UPDATE media_uploads SET status = 'deleted' WHERE id = \?`).WithArgs(int64(8)).WillReturnResult(sqlmock.NewResult(0, 1))
	if w := testutil.Request(r, http.MethodPost, "/v1/media/uploads/8/confirm", map[string]any{}, nil); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mismatch: %d %s", w.Code, w.Body)
	}
	if _, ok := mem.Objects["profile_photo/42/b.png"]; ok {
		t.Fatal("mismatched object must be deleted")
	}
}

func TestConfirmOtherUsersUploadIs404(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	r, routes := testutil.UserEngine(testutil.VerifiedUser(uid))
	New(d).Routes(routes)
	expectUpload(mock, 9, uid, "", "") // scoped by user_id: no row for the caller
	w := testutil.Request(r, http.MethodPost, "/v1/media/uploads/9/confirm", map[string]any{}, nil)
	if w.Code != http.StatusNotFound || decode(t, w.Body.Bytes()).Error.Code != "NOT_FOUND" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := testutil.Request(r, http.MethodPost, "/v1/media/uploads/abc/confirm", map[string]any{}, nil); w.Code == http.StatusOK {
		t.Fatal("bad id accepted")
	}
}

func TestAdminUpload(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	r, routes := testutil.AdminEngine(5, []string{"ads.write"})
	New(d).Routes(routes)

	mock.ExpectExec(`INSERT INTO media_uploads`).WithArgs(int64(0), "post_image", sqlmock.AnyArg(), "image/jpeg", int64(500)).WillReturnResult(sqlmock.NewResult(91, 1))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(int64(5), "media.upload_create", "media_upload", "91", `{"purpose":"post_image"}`, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	w := testutil.Request(r, http.MethodPost, "/v1/admin/media/uploads", map[string]any{"purpose": "post_image", "contentType": "image/jpeg", "sizeBytes": 500}, nil)
	if w.Code != http.StatusOK || !strings.Contains(decode(t, w.Body.Bytes()).Data.UploadURL, "post_image/0/") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	w = testutil.Request(r, http.MethodPost, "/v1/admin/media/uploads", map[string]any{"purpose": "profile_photo", "contentType": "image/jpeg", "sizeBytes": 500}, nil)
	if w.Code != http.StatusUnprocessableEntity || decode(t, w.Body.Bytes()).Error.Fields["purpose"] == "" {
		t.Fatalf("purpose: %d %s", w.Code, w.Body)
	}
}

func TestAdminConfirm(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	mem := d.Storage.(*testutil.MemStorage)
	r, routes := testutil.AdminEngine(5, []string{"ads.write"})
	New(d).Routes(routes)

	// Scoped to owner 0: a user's upload id is not found.
	expectUpload(mock, 9, 0, "", "")
	if w := testutil.Request(r, http.MethodPost, "/v1/admin/media/uploads/9/confirm", map[string]any{}, nil); w.Code != http.StatusNotFound {
		t.Fatalf("other owner: %d %s", w.Code, w.Body)
	}

	mem.Put("post_image/0/a.png", "image/png", 1000)
	expectUpload(mock, 91, 0, "post_image/0/a.png", "pending")
	mock.ExpectExec(`UPDATE media_uploads SET status = 'confirmed'`).WithArgs(int64(91), int64(0)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(int64(5), "media.upload_confirm", "media_upload", "91", nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	w := testutil.Request(r, http.MethodPost, "/v1/admin/media/uploads/91/confirm", map[string]any{}, nil)
	if w.Code != http.StatusOK || decode(t, w.Body.Bytes()).Data.Status != "confirmed" {
		t.Fatalf("confirm: %d %s", w.Code, w.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	r2, routes2 := testutil.AdminEngine(5, []string{"users.read"})
	New(d).Routes(routes2)
	if w := testutil.Request(r2, http.MethodPost, "/v1/admin/media/uploads/91/confirm", map[string]any{}, nil); w.Code != http.StatusForbidden {
		t.Fatalf("perm: %d %s", w.Code, w.Body)
	}
}

func TestAdminUploadNeedsPermission(t *testing.T) {
	d, _ := testutil.Deps(t, nil)
	r, routes := testutil.AdminEngine(5, []string{"users.read"})
	New(d).Routes(routes)
	w := testutil.Request(r, http.MethodPost, "/v1/admin/media/uploads", map[string]any{"purpose": "post_image", "contentType": "image/jpeg", "sizeBytes": 500}, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

type failingDelete struct{ *testutil.MemStorage }

func (failingDelete) Delete(context.Context, string) error { return errors.New("s3 down") }

var _ storage.Storage = failingDelete{}

func TestCleanupPending(t *testing.T) {
	d, mock := testutil.Deps(t, map[string]any{"media.pending_cleanup_hours": 6})
	d.Now = func() time.Time { return fixedNow }
	mem := d.Storage.(*testutil.MemStorage)
	mem.Put("chat_image/1/a.jpg", "image/jpeg", 10)
	m := New(d)
	job := m.Jobs()["media.cleanup_pending"]

	mock.ExpectQuery(`SELECT id, s3_key FROM media_uploads WHERE status = 'pending' AND created_at < \? AND id > \? ORDER BY id LIMIT \?`).
		WithArgs(fixedNow.Add(-6*time.Hour), int64(0), 500).
		WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}).AddRow(1, "chat_image/1/a.jpg").AddRow(2, "chat_image/1/b.jpg"))
	mock.ExpectExec(`DELETE FROM media_uploads WHERE id = \? AND status = 'pending'`).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM media_uploads WHERE id = \? AND status = 'pending'`).WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 0)) // confirmed meanwhile
	if err := job(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := mem.Objects["chat_image/1/a.jpg"]; ok {
		t.Fatal("object not deleted")
	}

	// Running again with nothing left is a no-op (idempotent).
	mock.ExpectQuery(`SELECT id, s3_key FROM media_uploads WHERE status = 'pending'`).WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}))
	if err := job(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Storage failure keeps the row for the next run.
	d.Storage = failingDelete{mem}
	mock.ExpectQuery(`SELECT id, s3_key FROM media_uploads WHERE status = 'pending'`).WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}).AddRow(3, "chat_image/1/c.jpg"))
	if err := job(context.Background()); err != nil {
		t.Fatal(err)
	}

	// A DB error is returned so the scheduler retries.
	d.Storage = mem
	mock.ExpectQuery(`SELECT id, s3_key FROM media_uploads WHERE status = 'pending'`).WillReturnError(errors.New("db down"))
	if err := job(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

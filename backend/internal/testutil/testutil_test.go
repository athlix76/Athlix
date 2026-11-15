package testutil

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMediaFlowWithDeps(t *testing.T) {
	d, mock := Deps(t, map[string]any{"media.image_max_mb": 5})
	mock.ExpectExec(`INSERT INTO media_uploads`).WithArgs(int64(42), "profile_photo", sqlmock.AnyArg(), "image/png", int64(1000)).WillReturnResult(sqlmock.NewResult(9, 1))
	tk, err := d.Media.CreateUpload(context.Background(), 42, "profile_photo", "image/png", 1000)
	if err != nil || tk.UploadID != 9 {
		t.Fatalf("ticket %+v err %v", tk, err)
	}
	if _, err := d.Media.CreateUpload(context.Background(), 42, "profile_photo", "application/x-sh", 10); err == nil {
		t.Fatal("expected content type rejection")
	}
}

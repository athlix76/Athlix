package media_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/testutil"
)

func TestPurgeDeletesObjectAndIsIdempotent(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	mem := d.Storage.(*testutil.MemStorage)
	mem.Put("location_proof/7/abc.pdf", "application/pdf", 100)

	mock.ExpectQuery("SELECT s3_key, status FROM media_uploads WHERE id = \\?").WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"s3_key", "status"}).AddRow("location_proof/7/abc.pdf", "attached"))
	mock.ExpectExec("UPDATE media_uploads SET status = 'deleted' WHERE id = \\?").WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := d.Media.Purge(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	if _, ok := mem.Objects["location_proof/7/abc.pdf"]; ok {
		t.Fatal("object not deleted")
	}

	mock.ExpectQuery("SELECT s3_key, status FROM media_uploads").WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"s3_key", "status"}).AddRow("location_proof/7/abc.pdf", "deleted"))
	if err := d.Media.Purge(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT s3_key, status FROM media_uploads").WithArgs(int64(6)).
		WillReturnRows(sqlmock.NewRows([]string{"s3_key", "status"}))
	if err := d.Media.Purge(context.Background(), 6); err != nil {
		t.Fatal("missing upload must be a no-op")
	}
}

func TestReviewerViewURLOnlyForRestrictedPurposes(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	mock.ExpectQuery("SELECT s3_key, purpose FROM media_uploads WHERE id = \\?").WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"s3_key", "purpose"}).AddRow("location_proof/7/abc.pdf", "location_proof"))
	if u := d.Media.ReviewerViewURL(context.Background(), 5); u == "" {
		t.Fatal("want a signed url")
	}
	mock.ExpectQuery("SELECT s3_key, purpose FROM media_uploads").WithArgs(int64(6)).
		WillReturnRows(sqlmock.NewRows([]string{"s3_key", "purpose"}).AddRow("profile_photo/7/x.jpg", "profile_photo"))
	if u := d.Media.ReviewerViewURL(context.Background(), 6); u != "" {
		t.Fatal("non-restricted purpose must not be signed here")
	}
}

func TestViewURLsNeverSignLocationProofs(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	mock.ExpectQuery("purpose <> 'location_proof'").WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}))
	if u := d.Media.ViewURL(context.Background(), 5); u != "" {
		t.Fatal(u)
	}
}

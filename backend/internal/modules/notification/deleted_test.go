package notification

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/events"
)

func TestUserDeletedRemovesDeviceTokens(t *testing.T) {
	m, mock, _, _ := setup(t, nil)
	h := m.EventHandlers()[events.UserDeleted]
	if h == nil {
		t.Fatal("notification must handle user.deleted")
	}
	const deleted = int64(42)
	ev := events.New(events.UserDeleted, deleted, events.UserPayload{UserID: deleted})
	q := `DELETE FROM device_tokens WHERE user_id = \?`

	// First delivery removes the tokens; a redelivery finds none and still succeeds.
	mock.ExpectExec(q).WithArgs(deleted).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(q).WithArgs(deleted).WillReturnResult(sqlmock.NewResult(0, 0))
	for i := 0; i < 2; i++ {
		if err := h(context.Background(), ev); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}

	// Malformed payload: dropped without touching the DB.
	if err := h(context.Background(), events.New(events.UserDeleted, 0, events.UserPayload{})); err != nil {
		t.Fatal("malformed payload must be dropped, got", err)
	}

	// DB error: returned so the event is retried.
	mock.ExpectExec(q).WithArgs(deleted).WillReturnError(errors.New("db down"))
	if err := h(context.Background(), ev); err == nil {
		t.Fatal("db errors must be returned for retry")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/events"
)

func TestUserDeletedHidesTheirMessages(t *testing.T) {
	e := newEnv(t, nil)
	h := e.m.EventHandlers()[events.UserDeleted]
	if h == nil {
		t.Fatal("chat must handle user.deleted")
	}
	const deleted = int64(42)
	ev := events.New(events.UserDeleted, deleted, events.UserPayload{UserID: deleted})
	q := `UPDATE messages SET status = 'deleted' WHERE sender_id = \? AND status = 'visible'`

	// Only the deleted user's visible messages; a redelivery matches nothing and succeeds.
	e.mock.ExpectExec(q).WithArgs(deleted).WillReturnResult(sqlmock.NewResult(0, 3))
	e.mock.ExpectExec(q).WithArgs(deleted).WillReturnResult(sqlmock.NewResult(0, 0))
	for i := 0; i < 2; i++ {
		if err := h(context.Background(), ev); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}

	if err := h(context.Background(), events.New(events.UserDeleted, 0, events.UserPayload{})); err != nil {
		t.Fatal("malformed payload must be dropped, got", err)
	}

	e.mock.ExpectExec(q).WithArgs(deleted).WillReturnError(errors.New("db down"))
	if err := h(context.Background(), ev); err == nil {
		t.Fatal("db errors must be returned for retry")
	}
	if err := e.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

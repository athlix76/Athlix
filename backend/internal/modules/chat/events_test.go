package chat

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/events"
)

// dispatch registers the module's handlers and dispatches ev the given number of times.
func (e *env) dispatch(ev events.Event, times int) {
	e.t.Helper()
	if !e.subscribed {
		e.subscribed = true
		for typ, h := range e.m.EventHandlers() {
			e.d.Dispatcher.On(typ, h)
		}
	}
	for i := 0; i < times; i++ {
		if err := e.d.Dispatcher.Dispatch(context.Background(), ev); err != nil {
			e.t.Fatalf("dispatch %d: %v", i, err)
		}
	}
}

func (e *env) expectLock(eventID, userID int64, convStatus string, count int, memberStatus any) {
	e.mock.ExpectQuery(qLockGroup).WithArgs(userID, eventID).WillReturnRows(
		sqlmock.NewRows([]string{"id", "status", "member_count", "m_status"}).AddRow(50, convStatus, count, memberStatus))
}

func TestEventPublishedCreatesGroupIdempotently(t *testing.T) {
	e := newEnv(t, nil)
	for i := 0; i < 2; i++ {
		e.mock.ExpectQuery(`SELECT organiser_id, title, sport_id, city, state, status FROM events WHERE id = \?`).WithArgs(int64(9)).
			WillReturnRows(sqlmock.NewRows([]string{"o", "t", "s", "c", "st", "status"}).AddRow(7, "Sunday 5K", 3, "Pune", "MH", "published"))
		e.mock.ExpectBegin()
		e.mock.ExpectExec(`INSERT INTO conversations \(type, title, owner_id, event_id.*ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID\(id\)`).
			WithArgs("Sunday 5K", int64(7), int64(9), int64(3), "Pune", "MH").WillReturnResult(sqlmock.NewResult(50, 1))
		e.mock.ExpectExec(`INSERT INTO conversation_members .*ON DUPLICATE KEY`).WithArgs(int64(50), int64(7), "owner").WillReturnResult(sqlmock.NewResult(0, 1))
		e.mock.ExpectExec(`UPDATE conversations SET member_count`).WithArgs(int64(50), int64(50)).WillReturnResult(sqlmock.NewResult(0, 1))
		e.mock.ExpectCommit()
	}
	e.dispatch(events.New(events.EventPublished, 7, events.EventPayload{EventID: 9, OrganiserID: 7}), 2)
}

func TestEventPublishedAutoCreateOff(t *testing.T) {
	e := newEnv(t, map[string]any{"event.auto_create_group": false})
	e.dispatch(events.New(events.EventPublished, 7, events.EventPayload{EventID: 9, OrganiserID: 7}), 1)
}

func TestRegistrationAddsMemberIdempotently(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.expectLock(9, 4, "active", 1, nil)
	e.mock.ExpectExec(`INSERT INTO conversation_members`).WithArgs(int64(50), int64(4), "member").WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`UPDATE conversations SET member_count`).WithArgs(int64(50), int64(50)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	// Second delivery: already an active member, nothing written.
	e.mock.ExpectBegin()
	e.expectLock(9, 4, "active", 2, "active")
	e.mock.ExpectCommit()

	ev := events.New(events.RegistrationCreated, 4, events.RegistrationPayload{RegistrationID: 1, EventID: 9, UserID: 4, OrganiserID: 7, Status: "registered"})
	e.dispatch(ev, 2)
}

func TestRegistrationSkips(t *testing.T) {
	t.Run("waitlisted", func(t *testing.T) {
		e := newEnv(t, nil)
		e.dispatch(events.New(events.RegistrationCreated, 4, events.RegistrationPayload{EventID: 9, UserID: 4, Status: "waitlisted"}), 1)
	})
	t.Run("group full", func(t *testing.T) {
		e := newEnv(t, map[string]any{"chat.group_max_members": 2})
		e.mock.ExpectBegin()
		e.expectLock(9, 4, "active", 2, nil)
		e.mock.ExpectCommit()
		e.dispatch(events.New(events.RegistrationPromoted, 4, events.RegistrationPayload{EventID: 9, UserID: 4, Status: "registered"}), 1)
	})
	t.Run("no group", func(t *testing.T) {
		e := newEnv(t, nil)
		e.mock.ExpectBegin()
		e.mock.ExpectQuery(qLockGroup).WillReturnRows(sqlmock.NewRows([]string{"id", "status", "member_count", "m_status"}))
		e.mock.ExpectCommit()
		e.dispatch(events.New(events.RegistrationPromoted, 4, events.RegistrationPayload{EventID: 9, UserID: 4}), 1)
	})
}

func TestWithdrawnRemovesMemberIdempotently(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.expectLock(9, 4, "active", 3, "active")
	e.mock.ExpectExec(`UPDATE conversation_members SET status = 'left'`).WithArgs(int64(50), int64(4)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`UPDATE conversations SET member_count`).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.mock.ExpectBegin()
	e.expectLock(9, 4, "active", 2, "left")
	e.mock.ExpectCommit()
	e.dispatch(events.New(events.RegistrationWithdrawn, 4, events.RegistrationPayload{EventID: 9, UserID: 4, Status: "withdrawn"}), 2)
}

func TestEventClosedArchivesIdempotently(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectExec(`UPDATE conversations SET status = 'archived' WHERE event_id = \?`).WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`UPDATE conversations SET status = 'archived' WHERE event_id = \?`).WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 0))
	e.mock.ExpectExec(`UPDATE conversations SET status = 'archived' WHERE event_id = \?`).WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 0))
	e.dispatch(events.New(events.EventCancelled, 7, events.EventPayload{EventID: 9}), 2)
	e.dispatch(events.New(events.EventCompleted, 7, events.EventPayload{EventID: 9}), 1)
}

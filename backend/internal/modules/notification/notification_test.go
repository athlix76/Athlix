package notification

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/push"
	"athlix/backend/internal/platform/settings"
	"athlix/backend/internal/testutil"
)

const me = int64(7)

var fixedNow = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

func setup(t *testing.T, cfg map[string]any) (*Module, sqlmock.Sqlmock, *push.Fake, *app.Deps) {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return fixedNow }
	m := New(d)
	f := &push.Fake{}
	m.SetPushSender(f)
	app.RegisterEventHandlers(d, []app.Module{m})
	return m, mock, f, d
}

func userServer(m *Module) http.Handler {
	r, routes := testutil.UserEngine(testutil.VerifiedUser(me))
	m.Routes(routes)
	return r
}

func expectName(mock sqlmock.Sqlmock, id int64, name string) {
	mock.ExpectQuery(`FROM users u`).WithArgs(id).WillReturnRows(
		sqlmock.NewRows([]string{"id", "dn", "pr", "vs", "photo", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"}).
			AddRow(id, name, "player", "verified", nil, nil, nil, nil, nil, nil, nil, nil, nil))
}

func expectInsert(mock sqlmock.Sqlmock, userID int64, typ, title, body, data string) {
	mock.ExpectExec(`INSERT INTO notifications \(user_id, type, title, body, data\) VALUES`).
		WithArgs(userID, typ, title, body, data).WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectPrefs(mock sqlmock.Sqlmock, rows *sqlmock.Rows, ids ...driver.Value) {
	if rows == nil {
		rows = sqlmock.NewRows([]string{"user_id", "notification_prefs"})
	}
	mock.ExpectQuery(`SELECT user_id, notification_prefs FROM user_settings WHERE user_id IN`).WithArgs(ids...).WillReturnRows(rows)
}

func expectDevices(mock sqlmock.Sqlmock, rows *sqlmock.Rows, ids ...driver.Value) {
	if rows == nil {
		rows = sqlmock.NewRows([]string{"id", "user_id", "token"})
	}
	mock.ExpectQuery(`SELECT id, user_id, token FROM device_tokens WHERE user_id IN`).WithArgs(ids...).WillReturnRows(rows)
}

func devRows() *sqlmock.Rows { return sqlmock.NewRows([]string{"id", "user_id", "token"}) }

func dispatch(t *testing.T, d *app.Deps, e events.Event) {
	t.Helper()
	if err := d.Dispatcher.Dispatch(context.Background(), e); err != nil {
		t.Fatalf("dispatch %s: %v", e.Type, err)
	}
}

// ---- devices ----

func TestRegisterDevice(t *testing.T) {
	m, mock, _, _ := setup(t, nil)
	srv := userServer(m)
	mock.ExpectExec(`INSERT INTO device_tokens \(user_id, platform, token, app_version\) VALUES .* ON DUPLICATE KEY UPDATE user_id = VALUES\(user_id\)`).
		WithArgs(me, "android", "tok-1", "1.2.0").WillReturnResult(sqlmock.NewResult(1, 1))
	w := testutil.Request(srv, "POST", "/v1/notifications/devices", map[string]any{"platform": "android", "token": " tok-1 ", "appVersion": "1.2.0"}, nil)
	if w.Code != 204 {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}

	mock.ExpectExec(`INSERT INTO device_tokens`).WithArgs(me, "ios", "tok-2", nil).WillReturnResult(sqlmock.NewResult(2, 1))
	if w := testutil.Request(srv, "POST", "/v1/notifications/devices", map[string]any{"platform": "ios", "token": "tok-2"}, nil); w.Code != 204 {
		t.Fatalf("status %d", w.Code)
	}

	for _, body := range []map[string]any{
		{"platform": "symbian", "token": "x"},
		{"platform": "web", "token": ""},
		{"platform": "web", "token": strings.Repeat("a", 513)},
		{"platform": "web", "token": "x", "appVersion": strings.Repeat("9", 17)},
	} {
		if w := testutil.Request(srv, "POST", "/v1/notifications/devices", body, nil); w.Code != 422 {
			t.Fatalf("body %v: status %d", body, w.Code)
		}
	}
}

func TestDeleteDeviceScopedToCaller(t *testing.T) {
	m, mock, _, _ := setup(t, nil)
	mock.ExpectExec(`DELETE FROM device_tokens WHERE token = \? AND user_id = \?`).WithArgs("tok-1", me).WillReturnResult(sqlmock.NewResult(0, 1))
	if w := testutil.Request(userServer(m), "DELETE", "/v1/notifications/devices", map[string]any{"token": "tok-1"}, nil); w.Code != 204 {
		t.Fatalf("status %d", w.Code)
	}
	if w := testutil.Request(userServer(m), "DELETE", "/v1/notifications/devices", map[string]any{"token": ""}, nil); w.Code != 422 {
		t.Fatalf("status %d", w.Code)
	}
}

// ---- list / read ----

func notifRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "type", "title", "body", "data", "read_at", "created_at"})
}

func TestListNotifications(t *testing.T) {
	m, mock, _, _ := setup(t, nil)
	srv := userServer(m)
	readAt := fixedNow.Add(-time.Hour)
	mock.ExpectQuery(`SELECT id, type, title, body, data, read_at, created_at FROM notifications WHERE user_id = \? ORDER BY id DESC LIMIT \?`).
		WithArgs(me, 3).WillReturnRows(notifRows().
		AddRow(30, "event.published", "Your event is live", "x", []byte(`{"eventId":9}`), nil, fixedNow).
		AddRow(29, "follow.created", "New follower", "y", []byte(`{"userId":8}`), readAt, fixedNow).
		AddRow(28, "broadcast", "Hi", "z", []byte(`{}`), nil, fixedNow))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM notifications WHERE user_id = \? AND read_at IS NULL`).WithArgs(me).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(5))
	w := testutil.Request(srv, "GET", "/v1/notifications?limit=2", nil, nil)
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var out struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			NextCursor  string `json:"nextCursor"`
			UnreadCount *int   `json:"unreadCount"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 2 || out.Meta.NextCursor == "" || out.Meta.UnreadCount == nil || *out.Meta.UnreadCount != 5 {
		t.Fatalf("unexpected page %s", w.Body)
	}
	first := out.Data[0]
	if v, ok := first["readAt"]; !ok || v != nil {
		t.Fatalf("readAt must be present and null: %v", first)
	}
	if first["data"].(map[string]any)["eventId"].(float64) != 9 || first["createdAt"] != "2026-11-13T10:00:00Z" {
		t.Fatalf("bad item %v", first)
	}
	if out.Data[1]["readAt"] != "2026-11-13T09:00:00Z" {
		t.Fatalf("readAt = %v", out.Data[1]["readAt"])
	}

	// Next page: keyset before the last id, still scoped by the caller.
	mock.ExpectQuery(`FROM notifications WHERE user_id = \? AND id < \? ORDER BY id DESC LIMIT \?`).WithArgs(me, 29, 3).WillReturnRows(notifRows())
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).WithArgs(me).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	w = testutil.Request(srv, "GET", "/v1/notifications?limit=2&cursor="+out.Meta.NextCursor, nil, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestMarkRead(t *testing.T) {
	m, mock, _, _ := setup(t, nil)
	srv := userServer(m)
	mock.ExpectExec(`UPDATE notifications SET read_at = \? WHERE user_id = \? AND read_at IS NULL AND id IN \(`).
		WithArgs(fixedNow, me, 3, 4).WillReturnResult(sqlmock.NewResult(0, 2))
	if w := testutil.Request(srv, "POST", "/v1/notifications/read", map[string]any{"ids": []int{3, 4, 3}}, nil); w.Code != 204 {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	mock.ExpectExec(`UPDATE notifications SET read_at = \? WHERE user_id = \? AND read_at IS NULL$`).
		WithArgs(fixedNow, me).WillReturnResult(sqlmock.NewResult(0, 9))
	if w := testutil.Request(srv, "POST", "/v1/notifications/read", map[string]any{"all": true}, nil); w.Code != 204 {
		t.Fatalf("status %d", w.Code)
	}
	many := make([]int, 101)
	for i := range many {
		many[i] = i + 1
	}
	for _, body := range []map[string]any{{}, {"ids": []int{}}, {"ids": many}, {"ids": []int{0}}} {
		if w := testutil.Request(srv, "POST", "/v1/notifications/read", body, nil); w.Code != 422 {
			t.Fatalf("body %v: status %d", body, w.Code)
		}
	}
}

// ---- event handlers ----

func TestAllMappedEventsHaveHandlers(t *testing.T) {
	m, _, _, _ := setup(t, nil)
	h := m.EventHandlers()
	for _, typ := range []string{
		events.VerificationCompleted, events.GuardianConsentGranted, events.FollowCreated,
		events.OrganiserApproved, events.OrganiserRejected, events.EventSubmitted, events.EventPublished,
		events.EventRejected, events.EventCancelled, events.EventUpdated, events.RegistrationCreated,
		events.RegistrationWithdrawn, events.RegistrationPromoted, events.RegistrationPaymentMark,
		events.ListingPaid, events.ListingRefunded, events.MessageCreated, events.PostLiked, events.PostCommented,
		events.ConsultationRequested, events.ConsultationResponded, events.SubscriptionActivated,
		events.SubscriptionExpired, events.CommunityJoinRequested, events.CommunityJoinApproved, EventBroadcastPush,
	} {
		if h[typ] == nil {
			t.Errorf("no handler for %s", typ)
		}
	}
	if m.Jobs()["notification.event_reminders"] == nil {
		t.Error("reminder job missing")
	}
}

func TestFollowCreatedRowAndPushOnce(t *testing.T) {
	_, mock, fake, d := setup(t, nil)
	e := events.New(events.FollowCreated, 7, events.FollowPayload{FollowerID: 7, FolloweeID: 8})

	expectName(mock, 7, "Ann")
	expectInsert(mock, 8, "follow.created", "New follower", "Ann started following you.", `{"userId":7}`)
	expectPrefs(mock, nil, int64(8)) // no settings row: defaults (social on)
	expectDevices(mock, devRows().AddRow(1, 8, "tok-a").AddRow(2, 8, "tok-b"), int64(8))
	dispatch(t, d, e)

	// Redelivery of the same event writes nothing and pushes nothing.
	expectName(mock, 7, "Ann")
	dispatch(t, d, e)

	got := fake.Messages()
	if len(got) != 2 || got[0].Title != "New follower" || got[0].Data["userId"] != "7" || got[0].Data["type"] != "follow.created" {
		t.Fatalf("pushes = %+v", got)
	}
}

func TestSelfFollowIgnored(t *testing.T) {
	_, _, fake, d := setup(t, nil)
	dispatch(t, d, events.New(events.FollowCreated, 7, events.FollowPayload{FollowerID: 7, FolloweeID: 7}))
	if len(fake.Messages()) != 0 {
		t.Fatal("unexpected push")
	}
}

func TestPrefsOffWritesRowWithoutPush(t *testing.T) {
	_, mock, fake, d := setup(t, nil)
	expectName(mock, 9, "Bo")
	expectInsert(mock, 8, "post.liked", "New like", "Bo liked your post.", `{"postId":44}`)
	expectPrefs(mock, sqlmock.NewRows([]string{"user_id", "notification_prefs"}).AddRow(8, []byte(`{"messages":true,"social":false}`)), int64(8))
	dispatch(t, d, events.New(events.PostLiked, 9, events.PostPayload{PostID: 44, AuthorID: 8, ActorID: 9}))
	if len(fake.Messages()) != 0 {
		t.Fatal("push sent although social is off")
	}

	// Liking your own post notifies nobody.
	dispatch(t, d, events.New(events.PostLiked, 8, events.PostPayload{PostID: 44, AuthorID: 8, ActorID: 8}))
}

func TestAccountNoticeSkipsPrefsAndDeletesDeadToken(t *testing.T) {
	_, mock, fake, d := setup(t, nil)
	fake.Fail = map[string]error{"dead": push.ErrUnregistered}
	expectInsert(mock, 8, "verification.completed", "You're verified", "Your identity is verified. You can now use all of Athlix.", `{}`)
	expectDevices(mock, devRows().AddRow(5, 8, "dead").AddRow(4, 8, "live"), int64(8))
	mock.ExpectExec(`DELETE FROM device_tokens WHERE id = \? AND user_id = \?`).WithArgs(5, 8).WillReturnResult(sqlmock.NewResult(0, 1))
	dispatch(t, d, events.New(events.VerificationCompleted, 8, events.VerificationPayload{UserID: 8, Status: "verified"}))
	if got := fake.Messages(); len(got) != 1 || got[0].Token != "live" {
		t.Fatalf("pushes = %+v", got)
	}
}

func TestInsertFailureReleasesDedupeForRetry(t *testing.T) {
	_, mock, _, d := setup(t, nil)
	e := events.New(events.GuardianConsentGranted, 0, events.UserPayload{UserID: 8})
	mock.ExpectExec(`INSERT INTO notifications`).WillReturnError(context.DeadlineExceeded)
	if err := d.Dispatcher.Dispatch(context.Background(), e); err == nil {
		t.Fatal("expected error so the event is retried")
	}
	expectInsert(mock, 8, "guardian.consent_granted", "Guardian consent received", "Your guardian approved your account.", `{}`)
	expectDevices(mock, nil, int64(8))
	dispatch(t, d, e)
}

func expectEvent(mock sqlmock.Sqlmock, id int64, title, status string, organiser int64) {
	mock.ExpectQuery(`SELECT title, status, organiser_id FROM events WHERE id = \?`).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"title", "status", "organiser_id"}).AddRow(title, status, organiser))
}

func TestEventCancelledNotifiesPlayersAndRemovedOrganiser(t *testing.T) {
	_, mock, fake, d := setup(t, nil)
	expectEvent(mock, 5, "City Cup", "removed", 50)
	mock.ExpectQuery(`SELECT user_id FROM event_registrations WHERE event_id = \? AND status IN \('registered','waitlisted'\)`).WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(8).AddRow(9))
	body := "“City Cup” has been cancelled."
	expectInsert(mock, 8, "event.cancelled", "Event cancelled", body, `{"eventId":5}`)
	expectInsert(mock, 9, "event.cancelled", "Event cancelled", body, `{"eventId":5}`)
	expectPrefs(mock, nil, int64(8), int64(9))
	expectDevices(mock, devRows().AddRow(1, 9, "tok-9"), int64(8), int64(9))
	expectInsert(mock, 50, "event.cancelled", "Your event was removed", "“City Cup” was removed by the Athlix team. Open the app for details.", `{"eventId":5}`)
	expectPrefs(mock, nil, int64(50))
	expectDevices(mock, nil, int64(50))
	dispatch(t, d, events.New(events.EventCancelled, 0, events.EventPayload{EventID: 5, OrganiserID: 50}))
	if got := fake.Messages(); len(got) != 1 || got[0].Token != "tok-9" || got[0].Data["eventId"] != "5" {
		t.Fatalf("pushes = %+v", got)
	}
}

func TestRegistrationCreatedNotifiesOrganiser(t *testing.T) {
	_, mock, _, d := setup(t, nil)
	expectEvent(mock, 5, "City Cup", "published", 50)
	expectName(mock, 8, "Ann")
	expectInsert(mock, 50, "registration.created", "New waitlist entry", "Ann joined the waitlist for “City Cup”.", `{"eventId":5,"userId":8}`)
	expectPrefs(mock, nil, int64(50))
	expectDevices(mock, nil, int64(50))
	dispatch(t, d, events.New(events.RegistrationCreated, 8, events.RegistrationPayload{RegistrationID: 1, EventID: 5, UserID: 8, OrganiserID: 50, Status: "waitlisted"}))
}

func TestCommunityJoinRequested(t *testing.T) {
	_, mock, _, d := setup(t, nil)
	mock.ExpectQuery(`SELECT type, COALESCE\(title, ''\), status FROM conversations WHERE id = \?`).WithArgs(12).
		WillReturnRows(sqlmock.NewRows([]string{"type", "title", "status"}).AddRow("community", "Kochi Runners", "active"))
	expectName(mock, 8, "Ann")
	expectInsert(mock, 3, "community.join_requested", "New join request", "Ann asked to join “Kochi Runners”.", `{"communityId":12,"conversationId":12,"userId":8}`)
	expectPrefs(mock, nil, int64(3))
	expectDevices(mock, nil, int64(3))
	dispatch(t, d, events.New(events.CommunityJoinRequested, 8, events.CommunityJoinPayload{ConversationID: 12, UserID: 8, OwnerID: 3}))
}

// ---- message.created ----

func expectConversation(mock sqlmock.Sqlmock, id int64, typ, title string) {
	mock.ExpectQuery(`FROM conversations WHERE id = \?`).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"type", "title", "status"}).AddRow(typ, title, "active"))
}

func expectMessage(mock sqlmock.Sqlmock, id int64, kind string) {
	mock.ExpectQuery(`SELECT kind, status FROM messages WHERE id = \?`).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "status"}).AddRow(kind, "visible"))
}

func expectMembers(mock sqlmock.Sqlmock, conv, sender int64, ids ...int64) {
	rows := sqlmock.NewRows([]string{"user_id"})
	for _, id := range ids {
		rows.AddRow(id)
	}
	// Muted members are excluded by the query itself (muted_until in the future).
	mock.ExpectQuery(`FROM conversation_members\s+WHERE conversation_id = \? AND status = 'active' AND user_id <> \? AND \(muted_until IS NULL OR muted_until <= \?\)`).
		WithArgs(conv, sender, fixedNow).WillReturnRows(rows)
}

func TestMessageCreatedPushesOfflineMembersOnly(t *testing.T) {
	_, mock, fake, d := setup(t, nil)
	ctx := context.Background()
	d.Redis.SAdd(ctx, "ws:user:9", "conn-1") // user 9 is online
	e := events.New(events.MessageCreated, 7, events.MessagePayload{MessageID: 100, ConversationID: 12, SenderID: 7})

	expectConversation(mock, 12, "direct", "")
	expectMessage(mock, 100, "text")
	expectMembers(mock, 12, 7, 8, 9, 10)
	expectName(mock, 7, "Ann")
	expectPrefs(mock, sqlmock.NewRows([]string{"user_id", "notification_prefs"}).AddRow(10, []byte(`{"messages":false}`)), int64(8), int64(10))
	expectDevices(mock, devRows().AddRow(1, 8, "tok-8"), int64(8))
	dispatch(t, d, e) // no notifications INSERT is expected: sqlmock fails on any

	got := fake.Messages()
	if len(got) != 1 || got[0].Token != "tok-8" || got[0].Title != "Ann" || got[0].Body != "sent you a message" || got[0].Data["conversationId"] != "12" {
		t.Fatalf("pushes = %+v", got)
	}

	// Redelivery: nothing new is pushed.
	expectConversation(mock, 12, "direct", "")
	expectMessage(mock, 100, "text")
	expectMembers(mock, 12, 7, 8, 9, 10)
	expectName(mock, 7, "Ann")
	dispatch(t, d, e)
	if len(fake.Messages()) != 1 {
		t.Fatal("duplicate push on redelivery")
	}
}

func TestMessageCreatedGroupTextAndAllOnline(t *testing.T) {
	_, mock, fake, d := setup(t, nil)
	expectConversation(mock, 12, "community", "Kochi Runners")
	expectMessage(mock, 101, "image")
	expectMembers(mock, 12, 7, 8)
	expectName(mock, 7, "Ann")
	expectPrefs(mock, nil, int64(8))
	expectDevices(mock, devRows().AddRow(1, 8, "tok-8"), int64(8))
	dispatch(t, d, events.New(events.MessageCreated, 7, events.MessagePayload{MessageID: 101, ConversationID: 12, SenderID: 7}))
	if got := fake.Messages(); len(got) != 1 || got[0].Title != "Kochi Runners" || got[0].Body != "Ann: sent a photo" {
		t.Fatalf("pushes = %+v", got)
	}

	// Everyone online: no name lookup, no push.
	d.Redis.SAdd(context.Background(), "ws:user:8", "c")
	expectConversation(mock, 12, "community", "Kochi Runners")
	expectMessage(mock, 102, "text")
	expectMembers(mock, 12, 7, 8)
	dispatch(t, d, events.New(events.MessageCreated, 7, events.MessagePayload{MessageID: 102, ConversationID: 12, SenderID: 7}))
	if len(fake.Messages()) != 1 {
		t.Fatal("pushed to an online member")
	}
}

func TestPushDisabledWithoutSecret(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	m := New(d) // no injected sender, no fcm secret in secrets.Static
	app.RegisterEventHandlers(d, []app.Module{m})
	expectInsert(mock, 8, "subscription.activated", "Your plan is active", "Your premium benefits are now on.", `{}`)
	expectDevices(mock, devRows().AddRow(1, 8, "tok"), int64(8))
	dispatch(t, d, events.New(events.SubscriptionActivated, 8, events.SubscriptionPayload{SubscriptionID: 1, UserID: 8, PlanID: 2}))
}

// ---- broadcast ----

func adminServer(m *Module, perms []string) http.Handler {
	r, routes := testutil.AdminEngine(1, perms)
	m.Routes(routes)
	return r
}

func TestBroadcast(t *testing.T) {
	m, mock, fake, _ := setup(t, nil)
	srv := adminServer(m, []string{"notifications.broadcast"})
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO notifications \(user_id, type, title, body, data\)\s+SELECT u.id, 'broadcast', \?, \?, JSON_OBJECT\(\) FROM users u.*u.is_minor = 0 AND EXISTS \(SELECT 1 FROM user_roles ur WHERE ur.user_id = u.id AND ur.role IN \(.*\)\) AND p.sport_id = \? AND p.state = \?`).
		WithArgs("Season opens", "Register now", "player", "coach", 3, "Kerala").WillReturnResult(sqlmock.NewResult(100, 3))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(id\), 0\) FROM notifications WHERE type = 'broadcast' AND id >= \?`).WithArgs(100).
		WillReturnRows(sqlmock.NewRows([]string{"m"}).AddRow(102))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(1, "notification.broadcast", "notification", nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	// Synchronous event bus: the push batch runs right after commit.
	mock.ExpectQuery(`SELECT id, user_id, title, body FROM notifications WHERE type = 'broadcast' AND id > \? AND id <= \? ORDER BY id LIMIT \?`).
		WithArgs(99, 102, 500).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "title", "body"}).
		AddRow(100, 8, "Season opens", "Register now").AddRow(101, 9, "Season opens", "Register now").AddRow(102, 10, "Season opens", "Register now"))
	expectPrefs(mock, sqlmock.NewRows([]string{"user_id", "notification_prefs"}).AddRow(8, []byte(`{"marketing":true}`)), int64(8), int64(9), int64(10))
	expectDevices(mock, devRows().AddRow(1, 8, "tok-8"), int64(8))

	w := testutil.Request(srv, "POST", "/v1/admin/notifications/broadcast", map[string]any{
		"title": "Season opens", "body": "Register now",
		"audience": map[string]any{"roles": []string{"player", "coach", "player"}, "sportId": 3, "state": "Kerala"},
	}, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"queued":3`) {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	if got := fake.Messages(); len(got) != 1 || got[0].Token != "tok-8" || got[0].Data["type"] != "broadcast" {
		t.Fatalf("pushes = %+v", got)
	}
}

func TestBroadcastNoRecipientsIncludeMinors(t *testing.T) {
	m, mock, _, _ := setup(t, nil)
	mock.ExpectBegin()
	mock.ExpectExec(`WHERE u.status = 'active' AND u.onboarding_step = 'done'$`).WithArgs("T", "B").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	w := testutil.Request(adminServer(m, []string{"*"}), "POST", "/v1/admin/notifications/broadcast",
		map[string]any{"title": "T", "body": "B", "audience": map[string]any{"includeMinors": true}}, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"queued":0`) {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestBroadcastPermissionAndValidation(t *testing.T) {
	m, _, _, _ := setup(t, nil)
	body := map[string]any{"title": "T", "body": "B", "audience": map[string]any{}}
	if w := testutil.Request(adminServer(m, []string{"communities.moderate"}), "POST", "/v1/admin/notifications/broadcast", body, nil); w.Code != 403 {
		t.Fatalf("status %d", w.Code)
	}
	srv := adminServer(m, []string{"notifications.broadcast"})
	for _, b := range []map[string]any{
		{"title": "", "body": "B"},
		{"title": strings.Repeat("t", 121), "body": "B"},
		{"title": "T", "body": strings.Repeat("b", 501)},
		{"title": "T", "body": "B", "audience": map[string]any{"roles": []string{"admin"}}},
		{"title": "T", "body": "B", "audience": map[string]any{"sportId": -1}},
	} {
		if w := testutil.Request(srv, "POST", "/v1/admin/notifications/broadcast", b, nil); w.Code != 422 {
			t.Fatalf("body %v: status %d", b, w.Code)
		}
	}
}

func TestBroadcastPushPagesAndDedupes(t *testing.T) {
	m, mock, _, _ := setup(t, nil)
	ctx := context.Background()
	rows := sqlmock.NewRows([]string{"id", "user_id", "title", "body"})
	ids := make([]driver.Value, 0, batchSize)
	for i := int64(1); i <= batchSize; i++ {
		rows.AddRow(i, 1000+i, "T", "B")
		ids = append(ids, 1000+i)
	}
	e := events.New(EventBroadcastPush, 0, broadcastPushPayload{FirstID: 1, LastID: 600, AfterID: 0})
	mock.ExpectQuery(`FROM notifications WHERE type = 'broadcast'`).WithArgs(0, 600, 500).WillReturnRows(rows)
	expectPrefs(mock, nil, ids...) // marketing defaults to off: no device lookup
	mock.ExpectQuery(`FROM notifications WHERE type = 'broadcast'`).WithArgs(500, 600, 500).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "title", "body"}).AddRow(501, 2000, "T", "B"))
	expectPrefs(mock, nil, int64(2000))
	if err := m.Service().onBroadcastPush(ctx, e); err != nil {
		t.Fatal(err)
	}

	// Redelivery of the first batch: rows are read to find the next page, but no prefs or
	// devices are loaded again (both batches are claimed).
	mock.ExpectQuery(`FROM notifications WHERE type = 'broadcast'`).WithArgs(0, 600, 500).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "title", "body"}).AddRow(1, 1001, "T", "B"))
	if err := m.Service().onBroadcastPush(ctx, e); err != nil {
		t.Fatal(err)
	}
}

// ---- reminders ----

func reminderRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"r.id", "event_id", "user_id", "title", "starts_at"})
}

func TestEventRemindersJob(t *testing.T) {
	m, mock, fake, d := setup(t, nil)
	job := m.Jobs()["notification.event_reminders"]
	ctx := context.Background()
	q := `FROM event_registrations r JOIN events e ON e.id = r.event_id\s+WHERE r.status = 'registered' AND e.status = 'published' AND e.starts_at > \? AND e.starts_at <= \? AND r.id > \?`
	starts := fixedNow.Add(5 * time.Hour)
	body := "“City Cup” starts in about 5 hours."

	mock.ExpectQuery(q).WithArgs(fixedNow, fixedNow.Add(24*time.Hour), 0, 500).WillReturnRows(reminderRows().
		AddRow(1, 5, 8, "City Cup", starts).AddRow(2, 5, 9, "City Cup", starts))
	expectInsert(mock, 8, TypeEventReminder, "Event reminder", body, `{"eventId":5}`)
	expectInsert(mock, 9, TypeEventReminder, "Event reminder", body, `{"eventId":5}`)
	expectPrefs(mock, nil, int64(8), int64(9))
	expectDevices(mock, devRows().AddRow(1, 8, "tok-8"), int64(8), int64(9))
	if err := job(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fake.Messages(); len(got) != 1 || got[0].Body != body {
		t.Fatalf("pushes = %+v", got)
	}

	// Re-run: same rows, already reminded, nothing written.
	mock.ExpectQuery(q).WithArgs(fixedNow, fixedNow.Add(24*time.Hour), 0, 500).WillReturnRows(reminderRows().
		AddRow(1, 5, 8, "City Cup", starts).AddRow(2, 5, 9, "City Cup", starts))
	if err := job(ctx); err != nil {
		t.Fatal(err)
	}

	// The window follows notification.event_reminder_hours.
	d.Settings.Set("notification.event_reminder_hours", 2)
	mock.ExpectQuery(q).WithArgs(fixedNow, fixedNow.Add(2*time.Hour), 0, 500).WillReturnRows(reminderRows())
	if err := job(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fake.Messages()) != 1 {
		t.Fatal("reminder repeated")
	}
	if ttl := d.Redis.TTL(ctx, "notif:reminder:5:8").Val(); ttl != 72*time.Hour {
		t.Fatalf("reminder dedupe ttl = %v", ttl)
	}
}

func TestReminderDefaultMatchesSeed(t *testing.T) {
	// The Go default must equal the seed value (migrations/0007_seed.sql: 24).
	st := settings.NewStatic(nil)
	if got := st.Int(context.Background(), "notification.event_reminder_hours", defaultReminderH); got != 24 {
		t.Fatalf("default = %d", got)
	}
}

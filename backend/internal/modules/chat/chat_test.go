package chat

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/testutil"
)

func TestSendMessageHappyPathFansOutAndDropsGone(t *testing.T) {
	e := newEnv(t, nil)
	e.connect("c1", 1)
	e.connect("c2", 2)
	e.connect("c2old", 2)
	e.fd.gone["c2old"] = true

	var published []events.MessagePayload
	e.d.Dispatcher.On(events.MessageCreated, func(_ context.Context, ev events.Event) error {
		var p events.MessagePayload
		_ = ev.Decode(&p)
		published = append(published, p)
		return nil
	})

	e.expectMember(10, 1, TypeDirect, "active", "1:2", "member")
	e.expectNoClientMsg(1, "c-1")
	e.expectCount(qBlocks, 0, int64(1), int64(2), int64(2), int64(1))
	e.expectSendTx(10, 1, 100, "hi there", "c-1")
	e.expectSummaries(1)
	e.expectActiveMembers(10, 1, 2)

	r := e.userEngine(adult(1))
	w := testutil.Request(r, http.MethodPost, "/v1/chat/conversations/10/messages",
		map[string]any{"clientMsgId": "c-1", "kind": "text", "body": "  hi there  "}, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	m := data(t, w)
	if m["id"].(float64) != 100 || m["body"] != "hi there" || m["clientMsgId"] != "c-1" || m["status"] != "visible" {
		t.Fatalf("message %v", m)
	}
	if got := e.fd.types("c2"); len(got) != 1 || got[0] != "message.new" {
		t.Fatalf("c2 frames %v", got)
	}
	if got := e.fd.types("c1"); len(got) != 1 { // REST send: the sender's other devices get it too
		t.Fatalf("c1 frames %v", got)
	}
	ctx := context.Background()
	if e.d.Redis.Exists(ctx, connKey("c2old")).Val() != 0 || e.d.Redis.SIsMember(ctx, userKey(2), "c2old").Val() {
		t.Fatal("gone connection was not cleaned up")
	}
	if !e.d.Redis.SIsMember(ctx, userKey(2), "c2").Val() {
		t.Fatal("live connection removed")
	}
	if len(published) != 1 || published[0].MessageID != 100 || published[0].ConversationID != 10 || published[0].SenderID != 1 {
		t.Fatalf("published %v", published)
	}
}

func TestSendImageMessage(t *testing.T) {
	e := newEnv(t, nil)
	e.expectMember(11, 1, TypeCommunity, "active", nil, "member")
	e.expectNoClientMsg(1, "img-1")
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(`FROM media_uploads WHERE id = \?`).WillReturnRows(
		sqlmock.NewRows([]string{"id", "user_id", "purpose", "s3_key", "content_type", "size_bytes", "status"}).
			AddRow(5, 1, "chat_image", "chat_image/1/a.jpg", "image/jpeg", 1000, "confirmed"))
	e.mock.ExpectExec(`UPDATE media_uploads SET status = 'attached'`).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(qInsertMsg).WithArgs(int64(11), int64(1), "image", nil, int64(5), nil, "img-1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(101, 1))
	e.mock.ExpectExec(qTouch).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(qMarkRead).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.expectSummaries(1)
	e.mock.ExpectQuery(qMediaView).WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}).AddRow(5, "chat_image/1/a.jpg"))
	e.expectActiveMembers(11, 1)

	msg, created, err := e.s.SendMessage(context.Background(), 1, 11, SendInput{ClientMsgID: "img-1", Kind: KindImage, MediaUploadID: 5})
	if err != nil || !created {
		t.Fatalf("created %v err %v", created, err)
	}
	if msg.ImageURL == nil || *msg.ImageURL != "https://storage.test/get/chat_image/1/a.jpg" || msg.Body != nil {
		t.Fatalf("message %+v", msg)
	}
}

func TestMediaDisabled(t *testing.T) {
	e := newEnv(t, map[string]any{"chat.media_enabled": false})
	_, _, err := e.s.SendMessage(context.Background(), 1, 11, SendInput{ClientMsgID: "img-1", Kind: KindImage, MediaUploadID: 5})
	he, ok := httpx.As(err)
	if !ok || he.Code != httpx.CodeValidation || he.Fields["kind"] == "" {
		t.Fatalf("err %v", err)
	}
}

func TestMessageLengthFollowsSetting(t *testing.T) {
	e := newEnv(t, map[string]any{"chat.message_max_length": 5})
	ctx := context.Background()
	_, _, err := e.s.SendMessage(ctx, 1, 11, SendInput{ClientMsgID: "a", Body: "hello!"})
	if he, ok := httpx.As(err); !ok || he.Fields["body"] == "" {
		t.Fatalf("expected body length error, got %v", err)
	}
	e.d.Settings.Set("chat.message_max_length", 10)
	if err := e.s.validateSend(ctx, SendInput{ClientMsgID: "a", Kind: KindText, Body: "hello!"}); err != nil {
		t.Fatalf("after raising the limit: %v", err)
	}
	if err := e.s.validateSend(ctx, SendInput{Kind: KindText, Body: "x"}); err == nil {
		t.Fatal("clientMsgId should be required")
	}
	if err := e.s.validateSend(ctx, SendInput{ClientMsgID: "a", Kind: "video", Body: "x"}); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestRateLimitFollowsSetting(t *testing.T) {
	e := newEnv(t, map[string]any{"chat.messages_per_minute": 1})
	ctx := context.Background()

	e.expectMember(11, 1, TypeCommunity, "active", nil, "member")
	e.expectNoClientMsg(1, "r-1")
	e.expectSendTx(11, 1, 200, "one", "r-1")
	e.expectSummaries(1)
	e.expectActiveMembers(11, 1)
	if _, _, err := e.s.SendMessage(ctx, 1, 11, SendInput{ClientMsgID: "r-1", Body: "one"}); err != nil {
		t.Fatal(err)
	}

	e.expectMember(11, 1, TypeCommunity, "active", nil, "member")
	e.expectNoClientMsg(1, "r-2")
	_, _, err := e.s.SendMessage(ctx, 1, 11, SendInput{ClientMsgID: "r-2", Body: "two"})
	if he, ok := httpx.As(err); !ok || he.Code != httpx.CodeRateLimited {
		t.Fatalf("expected 429, got %v", err)
	}

	// Raising the setting lets the next send through the limiter (it then fails on replyToId).
	e.d.Settings.Set("chat.messages_per_minute", 5)
	e.expectMember(11, 1, TypeCommunity, "active", nil, "member")
	e.expectNoClientMsg(1, "r-3")
	e.mock.ExpectQuery(qInConv).WithArgs(int64(999), int64(11)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, _, err = e.s.SendMessage(ctx, 1, 11, SendInput{ClientMsgID: "r-3", Body: "three", ReplyToID: 999})
	if he, ok := httpx.As(err); !ok || he.Fields["replyToId"] == "" {
		t.Fatalf("expected replyToId error, got %v", err)
	}
}

func TestClientMsgIDIdempotency(t *testing.T) {
	e := newEnv(t, nil)
	e.connect("c2", 2)
	fired := 0
	e.d.Dispatcher.On(events.MessageCreated, func(context.Context, events.Event) error { fired++; return nil })
	ctx := context.Background()

	e.expectMember(10, 1, TypeDirect, "active", "1:2", "member")
	e.mock.ExpectQuery(qByClient).WithArgs(int64(1), "c-1").WillReturnRows(row(msgColumns, msgRowOf(100, 10, 1, "hi", "c-1", "visible")))
	e.expectSummaries(1)
	msg, created, err := e.s.SendMessage(ctx, 1, 10, SendInput{ClientMsgID: "c-1", Body: "hi"})
	if err != nil || created || msg.ID != 100 {
		t.Fatalf("msg %+v created %v err %v", msg, created, err)
	}
	if len(e.fd.types("c2")) != 0 || fired != 0 {
		t.Fatal("a retry must not fan out or publish again")
	}

	// The same clientMsgId in another conversation is a conflict.
	e.expectMember(11, 1, TypeCommunity, "active", nil, "member")
	e.mock.ExpectQuery(qByClient).WithArgs(int64(1), "c-1").WillReturnRows(row(msgColumns, msgRowOf(100, 10, 1, "hi", "c-1", "visible")))
	_, _, err = e.s.SendMessage(ctx, 1, 11, SendInput{ClientMsgID: "c-1", Body: "hi"})
	if he, ok := httpx.As(err); !ok || he.Code != httpx.CodeConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestBlockedDirectSendRefused(t *testing.T) {
	e := newEnv(t, nil)
	e.expectMember(10, 1, TypeDirect, "active", "1:2", "member")
	e.expectNoClientMsg(1, "c-1")
	e.expectCount(qBlocks, 1)
	_, _, err := e.s.SendMessage(context.Background(), 1, 10, SendInput{ClientMsgID: "c-1", Body: "hi"})
	if !errors.Is(err, errCantMessage) {
		t.Fatalf("err %v", err)
	}
}

func TestClosedChatRefused(t *testing.T) {
	e := newEnv(t, nil)
	e.expectMember(12, 1, TypeEvent, "archived", nil, "member")
	e.expectNoClientMsg(1, "c-1")
	_, _, err := e.s.SendMessage(context.Background(), 1, 12, SendInput{ClientMsgID: "c-1", Body: "hi"})
	if he, ok := httpx.As(err); !ok || he.Code != httpx.CodeInvalidState {
		t.Fatalf("err %v", err)
	}
}

func TestNonMemberGets404(t *testing.T) {
	e := newEnv(t, nil)
	r := e.userEngine(adult(3))

	e.mock.ExpectQuery(qConv).WithArgs(int64(3), int64(10)).WillReturnRows(sqlmock.NewRows(convColumns))
	e.expectNotMember(10, 3)
	e.expectNotMember(10, 3)
	e.expectNotMember(10, 3)
	e.expectNotMember(10, 3)
	e.mock.ExpectQuery(qDeletable).WithArgs(int64(3), int64(100)).WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c", "d", "e"}))
	e.mock.ExpectQuery(`SELECT c\.id FROM conversations c`).WithArgs(int64(9), int64(3)).WillReturnRows(sqlmock.NewRows([]string{"id"}))

	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/v1/chat/conversations/10", nil},
		{http.MethodGet, "/v1/chat/conversations/10/messages", nil},
		{http.MethodPost, "/v1/chat/conversations/10/messages", map[string]any{"clientMsgId": "x", "body": "hi"}},
		{http.MethodPost, "/v1/chat/conversations/10/read", map[string]any{"messageId": 100}},
		{http.MethodGet, "/v1/chat/conversations/10/members", nil},
		{http.MethodDelete, "/v1/chat/messages/100", nil},
		{http.MethodGet, "/v1/events/9/group", nil},
		{http.MethodGet, "/v1/chat/conversations/abc", nil},
	}
	for _, c := range cases {
		w := testutil.Request(r, c.method, c.path, c.body, nil)
		if w.Code != http.StatusNotFound || errCode(t, w) != httpx.CodeNotFound {
			t.Fatalf("%s %s: %d %s", c.method, c.path, w.Code, w.Body)
		}
	}
}

func TestListConversationsPages(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectQuery(qConv).WithArgs(int64(1), int64(2)).WillReturnRows(row(convColumns,
		[]any{10, "direct", nil, nil, nil, 2, 100, "active", testNow, "1:2", "member", nil, 3},
		[]any{11, "community", "Runners", nil, nil, 40, nil, "active", testNow, nil, "member", nil, 0}))
	e.mock.ExpectQuery(qMsgsByID).WithArgs(int64(100)).WillReturnRows(row(msgColumns, msgRowOf(100, 10, 2, "yo", "x", "visible")))
	e.expectSummaries(1, 2)

	w := testutil.Request(e.userEngine(adult(1)), http.MethodGet, "/v1/chat/conversations?limit=1", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	b := body(t, w)
	items := b["data"].([]any)
	c := items[0].(map[string]any)
	if len(items) != 1 || c["unreadCount"].(float64) != 3 || c["otherUser"].(map[string]any)["id"].(float64) != 2 ||
		c["lastMessage"].(map[string]any)["body"] != "yo" {
		t.Fatalf("items %v", items)
	}
	if b["meta"].(map[string]any)["nextCursor"] == "" {
		t.Fatal("expected a next cursor")
	}
}

func TestDeleteMarkReadLeave(t *testing.T) {
	e := newEnv(t, nil)
	e.connect("c2", 2)
	r := e.userEngine(adult(1))

	// Sender deletes their own message.
	e.mock.ExpectQuery(qDeletable).WithArgs(int64(1), int64(100)).WillReturnRows(
		sqlmock.NewRows([]string{"a", "b", "c", "d", "e"}).AddRow(10, 1, "visible", "direct", "member"))
	e.mock.ExpectExec(`UPDATE messages SET status = 'deleted'`).WithArgs(int64(100)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.expectActiveMembers(10, 1, 2)
	if w := testutil.Request(r, http.MethodDelete, "/v1/chat/messages/100", nil, nil); w.Code != http.StatusNoContent {
		t.Fatalf("delete %d %s", w.Code, w.Body)
	}

	// Someone else's message in a direct chat: 404.
	e.mock.ExpectQuery(qDeletable).WithArgs(int64(1), int64(101)).WillReturnRows(
		sqlmock.NewRows([]string{"a", "b", "c", "d", "e"}).AddRow(10, 2, "visible", "direct", "member"))
	if w := testutil.Request(r, http.MethodDelete, "/v1/chat/messages/101", nil, nil); w.Code != http.StatusNotFound {
		t.Fatalf("delete other %d", w.Code)
	}

	// Mark read tells the other member.
	e.expectMember(10, 1, TypeDirect, "active", "1:2", "member")
	e.mock.ExpectQuery(qInConv).WithArgs(int64(101), int64(10)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(101))
	e.mock.ExpectExec(qMarkRead).WithArgs(int64(101), int64(10), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.expectActiveMembers(10, 1, 2)
	if w := testutil.Request(r, http.MethodPost, "/v1/chat/conversations/10/read", map[string]any{"messageId": 101}, nil); w.Code != http.StatusNoContent {
		t.Fatalf("read %d %s", w.Code, w.Body)
	}
	if got := e.fd.types("c2"); len(got) != 2 || got[0] != "message.deleted" || got[1] != "read" {
		t.Fatalf("c2 frames %v", got)
	}

	// A direct chat can't be left.
	e.expectMember(10, 1, TypeDirect, "active", "1:2", "member")
	if w := testutil.Request(r, http.MethodPost, "/v1/chat/conversations/10/leave", nil, nil); w.Code != http.StatusConflict {
		t.Fatalf("leave %d", w.Code)
	}

	// Leaving a group recounts members.
	e.expectMember(11, 1, TypeCommunity, "active", nil, "member")
	e.mock.ExpectBegin()
	e.mock.ExpectExec(`UPDATE conversation_members SET status = 'left'`).WithArgs(int64(11), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`UPDATE conversations SET member_count`).WithArgs(int64(11), int64(11)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	if w := testutil.Request(r, http.MethodPost, "/v1/chat/conversations/11/leave", nil, nil); w.Code != http.StatusNoContent {
		t.Fatalf("leave group %d %s", w.Code, w.Body)
	}
}

func TestAdminMessages(t *testing.T) {
	t.Run("no permission is 403", func(t *testing.T) {
		e := newEnv(t, nil)
		w := testutil.Request(e.adminEngine(9, nil), http.MethodGet, "/v1/admin/conversations/10/messages", nil, nil)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status %d", w.Code)
		}
	})
	t.Run("no open report is 404", func(t *testing.T) {
		e := newEnv(t, nil)
		e.mock.ExpectBegin()
		e.expectCount(`FROM reports r`, 0, int64(10), int64(10))
		e.mock.ExpectRollback()
		w := testutil.Request(e.adminEngine(9, []string{"moderation.read_messages"}), http.MethodGet, "/v1/admin/conversations/10/messages", nil, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
	})
	t.Run("open report: audited, raw bodies", func(t *testing.T) {
		e := newEnv(t, nil)
		e.mock.ExpectBegin()
		e.expectCount(`FROM reports r`, 1, int64(10), int64(10))
		e.mock.ExpectExec(`INSERT INTO admin_audit_log`).
			WithArgs(int64(9), "conversation.read_messages", "conversation", "10", sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))
		e.mock.ExpectCommit()
		e.mock.ExpectQuery(qMsgsPage).WithArgs(int64(10), int64(31)).WillReturnRows(row(msgColumns, msgRowOf(100, 10, 2, "rude words", "x", "hidden")))
		e.expectSummaries(2)
		w := testutil.Request(e.adminEngine(9, []string{"moderation.read_messages"}), http.MethodGet, "/v1/admin/conversations/10/messages", nil, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
		items := body(t, w)["data"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["body"] != "rude words" {
			t.Fatalf("items %v", items)
		}
	})
}

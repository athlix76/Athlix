package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/testutil"
)

type outFrame struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func decodeFrames(t *testing.T, frames [][]byte) []outFrame {
	t.Helper()
	out := make([]outFrame, len(frames))
	for i, b := range frames {
		if err := json.Unmarshal(b, &out[i]); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestTicketSingleUseAndTTLFromSetting(t *testing.T) {
	e := newEnv(t, map[string]any{"chat.ws_ticket_ttl_seconds": 30})
	ctx := context.Background()

	tk, err := e.s.IssueTicket(ctx, 1)
	if err != nil || tk.Ticket == "" || tk.URL != "ws://test/ws" || !tk.ExpiresAt.Equal(testNow.Add(30*time.Second)) {
		t.Fatalf("ticket %+v err %v", tk, err)
	}
	if ttl := e.d.Redis.TTL(ctx, ticketKey(tk.Ticket)).Val(); ttl != 30*time.Second {
		t.Fatalf("ttl %v", ttl)
	}
	e.expectUser(adult(1))
	if uid, err := e.s.ConsumeTicket(ctx, tk.Ticket); err != nil || uid != 1 {
		t.Fatalf("consume uid %d err %v", uid, err)
	}
	if _, err := e.s.ConsumeTicket(ctx, tk.Ticket); !errors.Is(err, ErrInvalidTicket) {
		t.Fatalf("second use: %v", err)
	}
	if _, err := e.s.ConsumeTicket(ctx, ""); !errors.Is(err, ErrInvalidTicket) {
		t.Fatalf("empty: %v", err)
	}

	e.d.Settings.Set("chat.ws_ticket_ttl_seconds", 90)
	tk2, _ := e.s.IssueTicket(ctx, 4)
	if ttl := e.d.Redis.TTL(ctx, ticketKey(tk2.Ticket)).Val(); ttl != 90*time.Second {
		t.Fatalf("ttl after setting change %v", ttl)
	}
	suspended := adult(4)
	suspended.Status = "suspended"
	e.expectUser(suspended)
	if _, err := e.s.ConsumeTicket(ctx, tk2.Ticket); !errors.Is(err, ErrInvalidTicket) {
		t.Fatalf("suspended user: %v", err)
	}
}

func TestTicketRoute(t *testing.T) {
	e := newEnv(t, nil)
	w := testutil.Request(e.userEngine(adult(1)), http.MethodPost, "/v1/chat/ws-ticket", nil, nil)
	if w.Code != http.StatusOK || data(t, w)["url"] != "ws://test/ws" || data(t, w)["ticket"] == "" {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestConnectionRegistry(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	e.connect("c1", 1)
	rd := e.d.Redis
	if rd.Get(ctx, "ws:conn:c1").Val() != "1" || !rd.SIsMember(ctx, "ws:user:1", "c1").Val() {
		t.Fatal("keys not written")
	}
	rd.Expire(ctx, "ws:conn:c1", time.Minute)
	frames := decodeFrames(t, e.s.HandleFrame(ctx, 1, "c1", []byte(`{"action":"ping"}`)))
	if len(frames) != 1 || frames[0].Type != "pong" {
		t.Fatalf("frames %v", frames)
	}
	if rd.TTL(ctx, "ws:conn:c1").Val() != connTTL || rd.TTL(ctx, "ws:user:1").Val() != connTTL {
		t.Fatal("ping did not refresh the TTL")
	}
	if err := e.s.RemoveConnection(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	if err := e.s.RemoveConnection(ctx, "c1"); err != nil {
		t.Fatalf("second remove: %v", err)
	}
	if rd.Exists(ctx, "ws:conn:c1").Val() != 0 || rd.SIsMember(ctx, "ws:user:1", "c1").Val() {
		t.Fatal("keys not removed")
	}

	e.connect("c9", 1)
	e.fd.gone["c9"] = true
	if err := e.s.SendToConnection(ctx, "c9", []byte(`{}`)); !errors.Is(err, ErrGone) {
		t.Fatalf("send %v", err)
	}
	if rd.Exists(ctx, "ws:conn:c9").Val() != 0 {
		t.Fatal("gone connection kept")
	}
}

func TestHandleFrameSendAckSkipsOrigin(t *testing.T) {
	e := newEnv(t, nil)
	e.connect("c1", 1)
	e.connect("c1b", 1)
	e.connect("c2", 2)
	e.expectUser(adult(1))
	e.expectMember(11, 1, TypeCommunity, "active", nil, "member")
	e.expectNoClientMsg(1, "w-1")
	e.expectSendTx(11, 1, 300, "hello", "w-1")
	e.expectSummaries(1)
	e.expectActiveMembers(11, 1, 2)

	raw := `{"action":"sendMessage","data":{"conversationId":11,"clientMsgId":"w-1","kind":"text","body":"hello"}}`
	frames := decodeFrames(t, e.s.HandleFrame(context.Background(), 1, "c1", []byte(raw)))
	if len(frames) != 1 || frames[0].Type != "ack" {
		t.Fatalf("frames %v", frames)
	}
	var ack ackData
	_ = json.Unmarshal(frames[0].Data, &ack)
	if ack.ClientMsgID != "w-1" || ack.Message.ID != 300 {
		t.Fatalf("ack %+v", ack)
	}
	if len(e.fd.types("c1")) != 0 || len(e.fd.types("c1b")) != 1 || len(e.fd.types("c2")) != 1 {
		t.Fatalf("fan-out c1=%v c1b=%v c2=%v", e.fd.types("c1"), e.fd.types("c1b"), e.fd.types("c2"))
	}
}

func TestHandleFrameErrors(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()

	e.expectUser(adult(1))
	e.expectNotMember(10, 1)
	raw := `{"action":"sendMessage","data":{"conversationId":10,"clientMsgId":"w-9","body":"hi"}}`
	frames := decodeFrames(t, e.s.HandleFrame(ctx, 1, "c1", []byte(raw)))
	var ed errorData
	_ = json.Unmarshal(frames[0].Data, &ed)
	if frames[0].Type != "error" || ed.Code != httpx.CodeNotFound || ed.ClientMsgID == nil || *ed.ClientMsgID != "w-9" {
		t.Fatalf("frame %s", frames[0].Data)
	}

	for _, raw := range []string{`not json`, `{"action":"dance"}`} {
		frames := decodeFrames(t, e.s.HandleFrame(ctx, 1, "c1", []byte(raw)))
		_ = json.Unmarshal(frames[0].Data, &ed)
		if frames[0].Type != "error" || ed.Code != httpx.CodeBadRequest {
			t.Fatalf("%s: %s", raw, frames[0].Data)
		}
	}
}

func TestHandleFrameTyping(t *testing.T) {
	e := newEnv(t, nil)
	e.connect("c1", 1)
	e.connect("c2", 2)
	e.expectMember(11, 1, TypeCommunity, "active", nil, "member")
	e.expectActiveMembers(11, 1, 2)
	out := e.s.HandleFrame(context.Background(), 1, "c1", []byte(`{"action":"typing","data":{"conversationId":11}}`))
	if len(out) != 0 || len(e.fd.types("c1")) != 0 {
		t.Fatalf("typing should not echo: %v", out)
	}
	if got := e.fd.types("c2"); len(got) != 1 || got[0] != "typing" {
		t.Fatalf("c2 %v", got)
	}
}

func TestLocalWebSocketRoute(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	m := New(d) // keeps the in-process hub
	e := &env{t: t, d: d, mock: mock, m: m, s: m.Service()}
	r := e.userEngine(adult(1))

	w := testutil.Request(r, http.MethodGet, "/v1/chat/ws?ticket=nope", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad ticket %d", w.Code)
	}

	srv := httptest.NewServer(r)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tk, err := e.s.IssueTicket(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	e.expectUser(adult(1))
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/chat/ws?ticket="+tk.Ticket, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"action":"ping"}`)); err != nil {
		t.Fatal(err)
	}
	_, b, err := c.Read(ctx)
	if err != nil || !strings.Contains(string(b), `"pong"`) {
		t.Fatalf("read %s err %v", b, err)
	}
	if n := d.Redis.SCard(ctx, "ws:user:1").Val(); n != 1 {
		t.Fatalf("connections %d", n)
	}
	c.Close(websocket.StatusNormalClosure, "")
	for i := 0; i < 100 && d.Redis.SCard(ctx, "ws:user:1").Val() != 0; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if d.Redis.SCard(ctx, "ws:user:1").Val() != 0 {
		t.Fatal("connection not removed on close")
	}
}

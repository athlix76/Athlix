package chat

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
)

// ErrInvalidTicket is returned for a missing, expired, used or otherwise unusable ticket.
var ErrInvalidTicket = errors.New("chat: invalid ws ticket")

func ticketKey(t string) string { return "ws:ticket:" + t }

// IssueTicket stores a random single-use ticket → userId for chat.ws_ticket_ttl_seconds.
func (s *Service) IssueTicket(ctx context.Context, userID int64) (Ticket, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return Ticket{}, err
	}
	t := base64.RawURLEncoding.EncodeToString(b)
	ttl := s.d.Settings.Seconds(ctx, "chat.ws_ticket_ttl_seconds", 60)
	if ttl <= 0 {
		ttl = time.Minute
	}
	if err := s.d.Redis.Set(ctx, ticketKey(t), strconv.FormatInt(userID, 10), ttl).Err(); err != nil {
		return Ticket{}, err
	}
	return Ticket{Ticket: t, URL: s.d.Cfg.WSPublicURL, ExpiresAt: s.d.Now().Add(ttl).UTC()}, nil
}

// ConsumeTicket redeems a ticket once (GETDEL) and checks the user may still use chat (same
// checks as the verified route group: active, onboarding done, not in maintenance).
func (s *Service) ConsumeTicket(ctx context.Context, ticket string) (int64, error) {
	if ticket == "" || len(ticket) > 128 {
		return 0, ErrInvalidTicket
	}
	v, err := s.d.Redis.GetDel(ctx, ticketKey(ticket)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, ErrInvalidTicket
	}
	if err != nil {
		return 0, err
	}
	uid, err := strconv.ParseInt(v, 10, 64)
	if err != nil || uid <= 0 {
		return 0, ErrInvalidTicket
	}
	if err := s.userMayChat(ctx, uid); err != nil {
		return 0, err
	}
	return uid, nil
}

// userMayChat mirrors authn.UserAuth + RequireOnboarded + the maintenance switch.
func (s *Service) userMayChat(ctx context.Context, userID int64) error {
	if s.d.Settings.Bool(ctx, "app.maintenance_mode", false) {
		return ErrInvalidTicket
	}
	u, err := s.d.Users.Get(ctx, userID)
	if err != nil {
		return ErrInvalidTicket
	}
	if u.Status != "active" || u.OnboardingStep != "done" {
		return ErrInvalidTicket
	}
	return nil
}

// ---- protocol (docs/api.md "WebSocket protocol") ----

type clientFrame struct {
	Action string          `json:"action"`
	Data   json.RawMessage `json:"data"`
}

type wsSend struct {
	ConversationID int64 `json:"conversationId"`
	SendInput
}

type wsConv struct {
	ConversationID int64 `json:"conversationId"`
	MessageID      int64 `json:"messageId"`
}

type ackData struct {
	ClientMsgID string  `json:"clientMsgId"`
	Message     Message `json:"message"`
}

type errorData struct {
	ClientMsgID *string `json:"clientMsgId"`
	Code        string  `json:"code"`
	Message     string  `json:"message"`
}

type typingData struct {
	ConversationID int64 `json:"conversationId"`
	UserID         int64 `json:"userId"`
}

// HandleFrame runs one client frame for userID on connection connID and returns the frames
// to send back to that connection (ack / error / pong). Fan-out to other members happens
// inside. Shared by the local route and cmd/ws.
func (s *Service) HandleFrame(ctx context.Context, userID int64, connID string, raw []byte) [][]byte {
	var f clientFrame
	if err := json.Unmarshal(raw, &f); err != nil {
		return [][]byte{errFrame(nil, httpx.BadRequest("Frame is not valid JSON."))}
	}
	switch f.Action {
	case "ping":
		if err := s.Touch(ctx, connID); err != nil && !errors.Is(err, ErrGone) {
			logger.From(ctx).Warn("chat ping", slog.String("err", err.Error()))
		}
		return [][]byte{encodeFrame("pong", nil)}

	case "sendMessage":
		var in wsSend
		if err := json.Unmarshal(f.Data, &in); err != nil {
			return [][]byte{errFrame(nil, httpx.BadRequest("Frame data is not valid."))}
		}
		cid := in.ClientMsgID
		var cidp *string
		if cid != "" {
			cidp = &cid
		}
		if err := s.userMayChat(ctx, userID); err != nil {
			return [][]byte{errFrame(cidp, httpx.Unauthenticated())}
		}
		in.OriginConnID = connID
		msg, _, err := s.SendMessage(ctx, userID, in.ConversationID, in.SendInput)
		if err != nil {
			return [][]byte{errFrame(cidp, err)}
		}
		return [][]byte{encodeFrame("ack", ackData{ClientMsgID: cid, Message: msg})}

	case "typing":
		var in wsConv
		if err := json.Unmarshal(f.Data, &in); err != nil {
			return [][]byte{errFrame(nil, httpx.BadRequest("Frame data is not valid."))}
		}
		m, err := s.member(ctx, in.ConversationID, userID)
		if err != nil {
			return [][]byte{errFrame(nil, err)}
		}
		if m.Status == "active" {
			s.fanOutToMembers(ctx, in.ConversationID, userID, "", encodeFrame("typing", typingData{in.ConversationID, userID}))
		}
		return nil

	case "markRead":
		var in wsConv
		if err := json.Unmarshal(f.Data, &in); err != nil {
			return [][]byte{errFrame(nil, httpx.BadRequest("Frame data is not valid."))}
		}
		if err := s.MarkRead(ctx, userID, in.ConversationID, in.MessageID); err != nil {
			return [][]byte{errFrame(nil, err)}
		}
		return nil
	}
	return [][]byte{errFrame(nil, httpx.BadRequest("Unknown action."))}
}

// errFrame maps an error to the error frame. Unknown errors are logged and shown generically.
func errFrame(clientMsgID *string, err error) []byte {
	e, ok := httpx.As(err)
	if !ok {
		e = httpx.Internal()
	}
	return encodeFrame("error", errorData{ClientMsgID: clientMsgID, Code: e.Code, Message: e.Message})
}

// ---- local dev WebSocket route (ENV=local only) ----

const wsReadLimit = 64 << 10

// serveLocalWS upgrades GET /v1/chat/ws?ticket= and runs the read loop on the in-process hub.
func (s *Service) serveLocalWS(c *gin.Context) {
	ctx := c.Request.Context()
	uid, err := s.ConsumeTicket(ctx, c.Query("ticket"))
	if err != nil {
		if !errors.Is(err, ErrInvalidTicket) {
			logger.From(ctx).Error("chat ws ticket", slog.String("err", err.Error()))
		}
		httpx.Fail(c, httpx.Unauthenticated())
		return
	}
	if s.hub == nil {
		httpx.Fail(c, httpx.NotFound("Route"))
		return
	}
	conn, err := websocket.Accept(c.Writer, c.Request, nil)
	if err != nil {
		return // Accept already wrote the HTTP error
	}
	defer conn.CloseNow()
	conn.SetReadLimit(wsReadLimit)

	connID := uuid.NewString()
	write := func(ctx context.Context, b []byte) error {
		wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := conn.Write(wctx, websocket.MessageText, b); err != nil {
			return ErrGone
		}
		return nil
	}
	s.hub.Register(connID, write)
	defer s.hub.Unregister(connID)
	if err := s.RegisterConnection(ctx, connID, uid); err != nil {
		logger.From(ctx).Error("chat ws register", slog.String("err", err.Error()))
		conn.Close(websocket.StatusInternalError, "try again")
		return
	}
	defer func() { _ = s.RemoveConnection(context.WithoutCancel(ctx), connID) }()

	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		for _, out := range s.HandleFrame(ctx, uid, connID, data) {
			if write(ctx, out) != nil {
				return
			}
		}
	}
}

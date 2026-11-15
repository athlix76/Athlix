package chat

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"athlix/backend/internal/platform/userdir"
)

// Conversation is the API shape from docs/api.md "Chat".
type Conversation struct {
	ID          int64            `json:"id"`
	Type        string           `json:"type"`
	Title       *string          `json:"title"`
	AvatarURL   *string          `json:"avatarUrl"`
	OtherUser   *userdir.Summary `json:"otherUser"`
	EventID     *int64           `json:"eventId"`
	MemberCount int              `json:"memberCount"`
	MyRole      string           `json:"myRole"`
	Muted       bool             `json:"muted"`
	UnreadCount int              `json:"unreadCount"`
	LastMessage *Message         `json:"lastMessage"`
	Status      string           `json:"status"`
	UpdatedAt   time.Time        `json:"updatedAt"`
}

// Message is the API shape from docs/api.md "Chat".
type Message struct {
	ID             int64            `json:"id"`
	ConversationID int64            `json:"conversationId"`
	Sender         *userdir.Summary `json:"sender"`
	Kind           string           `json:"kind"`
	Body           *string          `json:"body"`
	ImageURL       *string          `json:"imageUrl"`
	ReplyTo        *ReplyRef        `json:"replyTo"`
	ClientMsgID    *string          `json:"clientMsgId"`
	Status         string           `json:"status"`
	CreatedAt      time.Time        `json:"createdAt"`
}

// ReplyRef is the short preview of the message being replied to.
type ReplyRef struct {
	ID          int64  `json:"id"`
	SenderName  string `json:"senderName"`
	BodyPreview string `json:"bodyPreview"`
}

// Member is one row of GET /chat/conversations/:id/members.
type Member struct {
	User     userdir.Summary `json:"user"`
	Role     string          `json:"role"`
	JoinedAt time.Time       `json:"joinedAt"`
}

// Ticket is the response of POST /chat/ws-ticket.
type Ticket struct {
	Ticket    string    `json:"ticket"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// SendInput is the body of POST /chat/conversations/:id/messages and the `data` of the
// sendMessage WebSocket action.
type SendInput struct {
	ClientMsgID   string `json:"clientMsgId"`
	Kind          string `json:"kind"`
	Body          string `json:"body"`
	MediaUploadID int64  `json:"mediaUploadId"`
	ReplyToID     int64  `json:"replyToId"`

	// OriginConnID is the WebSocket connection that sent the message (it gets an ack, so the
	// message.new fan-out skips it). Never read from JSON.
	OriginConnID string `json:"-"`
}

// Message kinds and statuses.
const (
	KindText   = "text"
	KindImage  = "image"
	KindSystem = "system"

	TypeDirect    = "direct"
	TypeCommunity = "community"
	TypeEvent     = "event"

	replyPreviewRunes = 80
	eventTitleRunes   = 80
	clientMsgIDMax    = 64
)

// convRow is a conversation joined with the caller's membership.
type convRow struct {
	ID             int64
	Type           string
	Title          sql.NullString
	AvatarUploadID sql.NullInt64
	EventID        sql.NullInt64
	MemberCount    int
	LastMessageID  sql.NullInt64
	Status         string
	ActivityAt     time.Time
	DirectKey      sql.NullString
	Role           string
	MutedUntil     sql.NullTime
	Unread         int
}

// memberRow is the caller's active membership in a conversation.
type memberRow struct {
	ConversationID int64
	Type           string
	Status         string // conversation status
	DirectKey      sql.NullString
	Role           string
}

// msgRow is one messages row.
type msgRow struct {
	ID             int64
	ConversationID int64
	SenderID       sql.NullInt64
	Kind           string
	Body           sql.NullString
	MediaUploadID  sql.NullInt64
	ReplyToID      sql.NullInt64
	ClientMsgID    sql.NullString
	Status         string
	CreatedAt      time.Time
}

// directKey is "<min>:<max>" of the two user ids.
func directKey(a, b int64) string {
	if a > b {
		a, b = b, a
	}
	return strconv.FormatInt(a, 10) + ":" + strconv.FormatInt(b, 10)
}

// otherOf returns the other user id of a direct key, or 0.
func otherOf(key string, me int64) int64 {
	a, b, ok := strings.Cut(key, ":")
	if !ok {
		return 0
	}
	x, err1 := strconv.ParseInt(a, 10, 64)
	y, err2 := strconv.ParseInt(b, 10, 64)
	if err1 != nil || err2 != nil {
		return 0
	}
	switch me {
	case x:
		return y
	case y:
		return x
	}
	return 0
}

// truncateRunes cuts s to at most n runes.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func strPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

// UnreadCounts is GET /chat/unread-count (muted conversations are not counted).
type UnreadCounts struct {
	Conversations int `json:"conversations"`
	Messages      int `json:"messages"`
}

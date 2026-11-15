package chat

import (
	"context"
	"database/sql"
	"time"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/userdir"
)

// Service is the chat service shared by the REST handlers, the local WebSocket route and
// cmd/ws. Build it with NewService.
type Service struct {
	d        *app.Deps
	delivery Delivery
	hub      *LocalHub // set when delivery is the in-process hub
}

// NewService picks the delivery: the API Gateway Management API when WS_MANAGEMENT_URL is set
// and AWS is configured, else an in-process hub (local dev).
func NewService(d *app.Deps) *Service {
	s := &Service{d: d}
	if d.Cfg != nil && d.Cfg.WSManagementURL != "" && d.AWS != nil {
		s.delivery = NewAPIGatewayDelivery(*d.AWS, d.Cfg.WSManagementURL)
	} else {
		s.hub = NewLocalHub()
		s.delivery = s.hub
	}
	return s
}

// SetDelivery replaces the delivery (tests).
func (s *Service) SetDelivery(dl Delivery) {
	s.delivery = dl
	if h, ok := dl.(*LocalHub); ok {
		s.hub = h
	} else {
		s.hub = nil
	}
}

const (
	defaultMessagesLimit = 30
	defaultConvLimit     = 20
	defaultMembersLimit  = 50
)

var (
	errConvNotFound    = httpx.NotFound("Conversation")
	errMessageNotFound = httpx.NotFound("Message")
)

// lookup is the batch-resolved data needed to render messages and conversations.
type lookup struct {
	users   map[int64]userdir.Summary
	urls    map[int64]string
	replies map[int64]msgRow
}

// resolve batch-loads reply targets, user summaries and signed image URLs. raw=true (admin
// moderation) also signs images of deleted / hidden messages.
func (s *Service) resolve(ctx context.Context, msgs []msgRow, userIDs, mediaIDs []int64, raw bool) (lookup, error) {
	lk := lookup{replies: map[int64]msgRow{}}
	var replyIDs []int64
	seen := map[int64]bool{}
	for _, m := range msgs {
		if m.ReplyToID.Valid && !seen[m.ReplyToID.Int64] {
			seen[m.ReplyToID.Int64] = true
			replyIDs = append(replyIDs, m.ReplyToID.Int64)
		}
	}
	if len(replyIDs) > 0 {
		rs, err := messagesByID(ctx, s.d.DB, replyIDs)
		if err != nil {
			return lk, err
		}
		for _, r := range rs {
			lk.replies[r.ID] = r
		}
	}
	users := uniq{}
	for _, id := range userIDs {
		users.add(id)
	}
	media := uniq{}
	for _, id := range mediaIDs {
		media.add(id)
	}
	for _, m := range msgs {
		if m.SenderID.Valid {
			users.add(m.SenderID.Int64)
		}
		if m.MediaUploadID.Valid && (raw || m.Status == "visible") {
			media.add(m.MediaUploadID.Int64)
		}
		if r, ok := lk.replies[m.ReplyToID.Int64]; ok && r.SenderID.Valid {
			users.add(r.SenderID.Int64)
		}
	}
	var err error
	if lk.users, err = s.d.Users.Summaries(ctx, users.ids); err != nil {
		return lk, err
	}
	lk.urls = s.d.Media.ViewURLs(ctx, media.ids)
	return lk, nil
}

// toMessage renders one row. Deleted / hidden messages lose body and image unless raw.
func (lk lookup) toMessage(r msgRow, raw bool) Message {
	m := Message{ID: r.ID, ConversationID: r.ConversationID, Kind: r.Kind, Status: r.Status, CreatedAt: r.CreatedAt.UTC(), ClientMsgID: strPtr(r.ClientMsgID)}
	if r.SenderID.Valid {
		if u, ok := lk.users[r.SenderID.Int64]; ok {
			m.Sender = &u
		}
	}
	if raw || r.Status == "visible" {
		m.Body = strPtr(r.Body)
		if r.MediaUploadID.Valid {
			if u, ok := lk.urls[r.MediaUploadID.Int64]; ok {
				m.ImageURL = &u
			}
		}
	}
	if r.ReplyToID.Valid {
		if t, ok := lk.replies[r.ReplyToID.Int64]; ok && t.ConversationID == r.ConversationID {
			ref := &ReplyRef{ID: t.ID}
			if t.SenderID.Valid {
				ref.SenderName = lk.users[t.SenderID.Int64].DisplayName
			}
			if t.Status == "visible" {
				ref.BodyPreview = truncateRunes(t.Body.String, replyPreviewRunes)
			}
			m.ReplyTo = ref
		}
	}
	return m
}

// renderMessages resolves and renders a page of messages.
func (s *Service) renderMessages(ctx context.Context, rows []msgRow, raw bool) ([]Message, error) {
	lk, err := s.resolve(ctx, rows, nil, nil, raw)
	if err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, lk.toMessage(r, raw))
	}
	return out, nil
}

// renderConversations resolves last messages, other users and avatars.
func (s *Service) renderConversations(ctx context.Context, me int64, rows []convRow) ([]Conversation, error) {
	var lastIDs, userIDs, mediaIDs []int64
	for _, r := range rows {
		if r.LastMessageID.Valid {
			lastIDs = append(lastIDs, r.LastMessageID.Int64)
		}
		if r.Type == TypeDirect {
			if o := otherOf(r.DirectKey.String, me); o > 0 {
				userIDs = append(userIDs, o)
			}
		}
		if r.AvatarUploadID.Valid {
			mediaIDs = append(mediaIDs, r.AvatarUploadID.Int64)
		}
	}
	last, err := messagesByID(ctx, s.d.DB, lastIDs)
	if err != nil {
		return nil, err
	}
	lk, err := s.resolve(ctx, last, userIDs, mediaIDs, false)
	if err != nil {
		return nil, err
	}
	byID := map[int64]msgRow{}
	for _, m := range last {
		byID[m.ID] = m
	}
	now := s.d.Now()
	out := make([]Conversation, 0, len(rows))
	for _, r := range rows {
		c := Conversation{ID: r.ID, Type: r.Type, Title: strPtr(r.Title), MemberCount: r.MemberCount, MyRole: r.Role,
			Muted: r.MutedUntil.Valid && r.MutedUntil.Time.After(now), UnreadCount: r.Unread, Status: r.Status, UpdatedAt: r.ActivityAt.UTC()}
		if r.EventID.Valid {
			id := r.EventID.Int64
			c.EventID = &id
		}
		if r.AvatarUploadID.Valid {
			if u, ok := lk.urls[r.AvatarUploadID.Int64]; ok {
				c.AvatarURL = &u
			}
		}
		if r.Type == TypeDirect {
			if u, ok := lk.users[otherOf(r.DirectKey.String, me)]; ok {
				c.OtherUser = &u
			}
		}
		if m, ok := byID[r.LastMessageID.Int64]; ok && r.LastMessageID.Valid {
			msg := lk.toMessage(m, false)
			c.LastMessage = &msg
		}
		out = append(out, c)
	}
	return out, nil
}

// Conversations pages the caller's conversations (latest activity first).
func (s *Service) Conversations(ctx context.Context, userID int64, p httpx.PageParams) ([]Conversation, string, error) {
	var before time.Time
	if p.Cursor.ID > 0 {
		t, err := time.Parse(time.RFC3339Nano, p.Cursor.Time)
		if err != nil {
			return nil, "", httpx.Validation(map[string]string{"cursor": "is invalid"})
		}
		before = t
	}
	rows, err := listConversations(ctx, s.d.DB, userID, before, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		l := rows[len(rows)-1]
		next = httpx.EncodeCursor(httpx.Cursor{ID: l.ID, Time: l.ActivityAt.UTC().Format(time.RFC3339Nano)})
	}
	out, err := s.renderConversations(ctx, userID, rows)
	return out, next, err
}

// Conversation returns one conversation the caller is an active member of (else 404).
func (s *Service) Conversation(ctx context.Context, userID, convID int64) (Conversation, error) {
	r, err := getConversation(ctx, s.d.DB, userID, convID)
	if db.IsNoRows(err) {
		return Conversation{}, errConvNotFound
	}
	if err != nil {
		return Conversation{}, err
	}
	out, err := s.renderConversations(ctx, userID, []convRow{r})
	if err != nil {
		return Conversation{}, err
	}
	return out[0], nil
}

// EventGroup returns the group of an event when the caller is a member (else 404).
func (s *Service) EventGroup(ctx context.Context, userID, eventID int64) (Conversation, error) {
	id, err := eventConversationID(ctx, s.d.DB, userID, eventID)
	if db.IsNoRows(err) {
		return Conversation{}, errConvNotFound
	}
	if err != nil {
		return Conversation{}, err
	}
	return s.Conversation(ctx, userID, id)
}

// member returns the caller's active membership or 404.
func (s *Service) member(ctx context.Context, convID, userID int64) (memberRow, error) {
	m, err := getMembership(ctx, s.d.DB, convID, userID)
	if db.IsNoRows(err) {
		return m, errConvNotFound
	}
	return m, err
}

// Messages pages messages newest first (id < before).
func (s *Service) Messages(ctx context.Context, userID, convID, before int64, limit int) ([]Message, string, error) {
	if _, err := s.member(ctx, convID, userID); err != nil {
		return nil, "", err
	}
	return s.messagesPage(ctx, convID, before, limit, false)
}

func (s *Service) messagesPage(ctx context.Context, convID, before int64, limit int, raw bool) ([]Message, string, error) {
	rows, err := messagesPage(ctx, s.d.DB, convID, before, limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	out, err := s.renderMessages(ctx, rows, raw)
	return out, next, err
}

type readData struct {
	ConversationID int64 `json:"conversationId"`
	UserID         int64 `json:"userId"`
	MessageID      int64 `json:"messageId"`
}

// MarkRead moves the caller's read marker forward and tells the other members.
func (s *Service) MarkRead(ctx context.Context, userID, convID, messageID int64) error {
	if _, err := s.member(ctx, convID, userID); err != nil {
		return err
	}
	if messageID <= 0 {
		return httpx.Validation(map[string]string{"messageId": "is required"})
	}
	ok, err := messageInConversation(ctx, s.d.DB, messageID, convID)
	if err != nil {
		return err
	}
	if !ok {
		return errMessageNotFound
	}
	if err := markRead(ctx, s.d.DB, convID, userID, messageID); err != nil {
		return err
	}
	s.fanOutToMembers(ctx, convID, userID, "", encodeFrame("read", readData{convID, userID, messageID}))
	return nil
}

// Mute sets or clears muted_until.
func (s *Service) Mute(ctx context.Context, userID, convID int64, until *time.Time) error {
	if _, err := s.member(ctx, convID, userID); err != nil {
		return err
	}
	if until != nil && !until.After(s.d.Now()) {
		until = nil // a time in the past means "unmuted"
	}
	return setMute(ctx, s.d.DB, convID, userID, until)
}

// Leave removes the caller from a group. Not for direct chats; the owner must hand over first.
func (s *Service) Leave(ctx context.Context, userID, convID int64) error {
	m, err := s.member(ctx, convID, userID)
	if err != nil {
		return err
	}
	if m.Type == TypeDirect {
		return httpx.InvalidState("You can't leave a direct chat.")
	}
	if m.Role == "owner" {
		return httpx.InvalidState("Hand over or delete the group before leaving.")
	}
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		left, err := setMemberLeft(ctx, tx, convID, userID)
		if err != nil || !left {
			return err
		}
		return recountMembers(ctx, tx, convID)
	})
}

// Members pages the active members of a conversation the caller belongs to.
func (s *Service) Members(ctx context.Context, userID, convID int64, p httpx.PageParams) ([]Member, string, error) {
	if _, err := s.member(ctx, convID, userID); err != nil {
		return nil, "", err
	}
	rows, err := listMembers(ctx, s.d.DB, convID, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].UserID})
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	sums, err := s.d.Users.Summaries(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	out := make([]Member, 0, len(rows))
	for _, r := range rows {
		u, ok := sums[r.UserID]
		if !ok {
			continue // deleted account
		}
		out = append(out, Member{User: u, Role: r.Role, JoinedAt: r.JoinedAt.UTC()})
	}
	return out, next, nil
}

type deletedData struct {
	ConversationID int64 `json:"conversationId"`
	MessageID      int64 `json:"messageId"`
}

// DeleteMessage soft-deletes a message: the sender, or an owner/admin of a group.
func (s *Service) DeleteMessage(ctx context.Context, userID, messageID int64) error {
	r, err := getDeletable(ctx, s.d.DB, messageID, userID)
	if db.IsNoRows(err) {
		return errMessageNotFound
	}
	if err != nil {
		return err
	}
	isSender := r.SenderID.Valid && r.SenderID.Int64 == userID
	isMod := r.ConvType != TypeDirect && (r.Role == "owner" || r.Role == "admin")
	if !isSender && !isMod {
		return errMessageNotFound
	}
	changed, err := softDeleteMessage(ctx, s.d.DB, messageID)
	if err != nil || !changed {
		return err
	}
	s.fanOutToMembers(ctx, r.ConversationID, 0, "", encodeFrame("message.deleted", deletedData{r.ConversationID, messageID}))
	return nil
}

// uniq collects distinct positive ids in insertion order.
type uniq struct {
	ids  []int64
	seen map[int64]bool
}

func (u *uniq) add(id int64) {
	if id <= 0 {
		return
	}
	if u.seen == nil {
		u.seen = map[int64]bool{}
	}
	if !u.seen[id] {
		u.seen[id] = true
		u.ids = append(u.ids, id)
	}
}

// UnreadCounts is the badge for the chat tab: conversations with unread messages and the
// total of those messages, across the caller's active, unmuted conversations.
func (s *Service) UnreadCounts(ctx context.Context, userID int64) (UnreadCounts, error) {
	c, m, err := unreadTotals(ctx, s.d.DB, userID, s.d.Now().UTC())
	return UnreadCounts{Conversations: c, Messages: m}, err
}

package chat

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/media"
)

// SendMessage is the ONE send path (REST, local WebSocket and cmd/ws). It checks membership
// and limits, saves the message, fans it out to online members and publishes
// message.created after commit. The same (sender, clientMsgId) returns the existing message
// with created=false and no fan-out or event.
func (s *Service) SendMessage(ctx context.Context, userID, convID int64, in SendInput) (Message, bool, error) {
	in.ClientMsgID = strings.TrimSpace(in.ClientMsgID)
	in.Body = strings.TrimSpace(in.Body)
	if in.Kind == "" {
		in.Kind = KindText
	}
	if err := s.validateSend(ctx, in); err != nil {
		return Message{}, false, err
	}

	m, err := s.member(ctx, convID, userID)
	if err != nil {
		return Message{}, false, err
	}

	if existing, err := messageByClientID(ctx, s.d.DB, userID, in.ClientMsgID); err == nil {
		return s.existing(ctx, existing, convID)
	} else if !db.IsNoRows(err) {
		return Message{}, false, err
	}

	if m.Status != "active" {
		return Message{}, false, httpx.InvalidState("This chat is closed.")
	}
	if m.Type == TypeDirect {
		other := otherOf(m.DirectKey.String, userID)
		blocked, err := s.d.Users.BlockedEitherWay(ctx, userID, other)
		if err != nil {
			return Message{}, false, err
		}
		if blocked {
			return Message{}, false, errCantMessage
		}
	}

	limit := s.d.Settings.Int(ctx, "chat.messages_per_minute", 30)
	ok, err := s.d.Limiter.Allow(ctx, "chat:"+strconv.FormatInt(userID, 10), limit, time.Minute)
	if err != nil {
		return Message{}, false, err
	}
	if !ok {
		return Message{}, false, httpx.RateLimited("You're sending messages too fast. Please wait a moment.")
	}

	if in.ReplyToID > 0 {
		ok, err := messageInConversation(ctx, s.d.DB, in.ReplyToID, convID)
		if err != nil {
			return Message{}, false, err
		}
		if !ok {
			return Message{}, false, httpx.Validation(map[string]string{"replyToId": "is not a message in this chat"})
		}
	}

	row := msgRow{
		ConversationID: convID,
		SenderID:       sql.NullInt64{Int64: userID, Valid: true},
		Kind:           in.Kind,
		Body:           sql.NullString{String: in.Body, Valid: in.Body != ""},
		MediaUploadID:  sql.NullInt64{Int64: in.MediaUploadID, Valid: in.Kind == KindImage},
		ReplyToID:      sql.NullInt64{Int64: in.ReplyToID, Valid: in.ReplyToID > 0},
		ClientMsgID:    sql.NullString{String: in.ClientMsgID, Valid: true},
		Status:         "visible",
		CreatedAt:      s.d.Now().UTC().Truncate(time.Millisecond),
	}
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if in.Kind == KindImage {
			if err := s.d.Media.Claim(ctx, tx, userID, in.MediaUploadID, media.PurposeChatImage); err != nil {
				if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrNotUploaded) || errors.Is(err, media.ErrMismatch) {
					return httpx.Validation(map[string]string{"mediaUploadId": "is not a chat image upload of yours"})
				}
				return err
			}
		}
		id, err := insertMessage(ctx, tx, row)
		if err != nil {
			return err
		}
		row.ID = id
		if err := touchConversation(ctx, tx, convID, id, row.CreatedAt); err != nil {
			return err
		}
		return markRead(ctx, tx, convID, userID, id)
	})
	if db.IsDuplicate(err) { // concurrent retry with the same clientMsgId won the insert
		existing, ferr := messageByClientID(ctx, s.d.DB, userID, in.ClientMsgID)
		if ferr != nil {
			return Message{}, false, ferr
		}
		return s.existing(ctx, existing, convID)
	}
	if err != nil {
		return Message{}, false, err
	}

	msgs, err := s.renderMessages(ctx, []msgRow{row}, false)
	if err != nil {
		return Message{}, false, err
	}
	msg := msgs[0]
	s.fanOutToMembers(ctx, convID, 0, in.OriginConnID, encodeFrame("message.new", msg))
	if err := s.d.Events.Publish(ctx, events.New(events.MessageCreated, userID,
		events.MessagePayload{MessageID: msg.ID, ConversationID: convID, SenderID: userID})); err != nil {
		logger.From(ctx).Error("publish message.created", slog.Int64("messageId", msg.ID), slog.String("err", err.Error()))
	}
	return msg, true, nil
}

// existing returns a message already sent with the same clientMsgId.
func (s *Service) existing(ctx context.Context, r msgRow, convID int64) (Message, bool, error) {
	if r.ConversationID != convID {
		return Message{}, false, httpx.Conflict("This clientMsgId was already used in another chat.")
	}
	msgs, err := s.renderMessages(ctx, []msgRow{r}, false)
	if err != nil {
		return Message{}, false, err
	}
	return msgs[0], false, nil
}

// validateSend checks the request shape and config-driven limits (no I/O except settings).
func (s *Service) validateSend(ctx context.Context, in SendInput) error {
	f := map[string]string{}
	if in.ClientMsgID == "" {
		f["clientMsgId"] = "is required"
	} else if len(in.ClientMsgID) > clientMsgIDMax {
		f["clientMsgId"] = "must be at most 64 characters"
	}
	maxLen := s.d.Settings.Int(ctx, "chat.message_max_length", 2000)
	switch in.Kind {
	case KindText:
		if in.Body == "" {
			f["body"] = "is required"
		}
		if in.MediaUploadID != 0 {
			f["mediaUploadId"] = "is only for image messages"
		}
	case KindImage:
		if !s.d.Settings.Bool(ctx, "chat.media_enabled", true) {
			f["kind"] = "images are turned off in chat"
		} else if in.MediaUploadID <= 0 {
			f["mediaUploadId"] = "is required for an image"
		}
	default:
		f["kind"] = "must be text or image"
	}
	if utf8.RuneCountInString(in.Body) > maxLen {
		f["body"] = "must be at most " + strconv.Itoa(maxLen) + " characters"
	}
	if in.ReplyToID < 0 {
		f["replyToId"] = "is invalid"
	}
	if len(f) > 0 {
		return httpx.Validation(f)
	}
	return nil
}

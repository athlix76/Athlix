package chat

import (
	"context"
	"database/sql"
	"log/slog"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/logger"
)

// Event consumers. Every handler is idempotent (SQS delivers at least once): inserts use
// unique keys, updates are guarded by status, and member_count is recounted, never added to.

func (s *Service) handlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.EventPublished:        s.onEventPublished,
		events.RegistrationCreated:   s.onRegistrationAdded,
		events.RegistrationPromoted:  s.onRegistrationAdded,
		events.RegistrationWithdrawn: s.onRegistrationWithdrawn,
		events.EventCancelled:        s.onEventClosed,
		events.EventCompleted:        s.onEventClosed,
		events.UserDeleted:           s.onUserDeleted,
	}
}

// onUserDeleted hides every message the deleted user sent (docs/security.md, account
// deletion: "posts/messages hidden"). Idempotent: a second run matches nothing.
func (s *Service) onUserDeleted(ctx context.Context, e events.Event) error {
	var p events.UserPayload
	if err := e.Decode(&p); err != nil || p.UserID <= 0 {
		return nil // malformed: nothing to retry
	}
	return hideUserMessages(ctx, s.d.DB, p.UserID)
}

// onEventPublished creates the event group (if event.auto_create_group) with the organiser
// as owner.
func (s *Service) onEventPublished(ctx context.Context, e events.Event) error {
	var p events.EventPayload
	if err := e.Decode(&p); err != nil || p.EventID <= 0 {
		logger.From(ctx).Warn("chat: bad event.published payload")
		return nil
	}
	if !s.d.Settings.Bool(ctx, "event.auto_create_group", true) {
		return nil
	}
	ev, err := getEvent(ctx, s.d.DB, p.EventID)
	if db.IsNoRows(err) {
		logger.From(ctx).Warn("chat: published event not found", slog.Int64("eventId", p.EventID))
		return nil
	}
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		id, err := upsertEventGroup(ctx, tx, p.EventID, ev)
		if err != nil {
			return err
		}
		if err := addMember(ctx, tx, id, ev.OrganiserID, "owner"); err != nil {
			return err
		}
		return recountMembers(ctx, tx, id)
	})
}

// onRegistrationAdded adds a registered (or promoted) player to the event group.
func (s *Service) onRegistrationAdded(ctx context.Context, e events.Event) error {
	var p events.RegistrationPayload
	if err := e.Decode(&p); err != nil || p.EventID <= 0 || p.UserID <= 0 {
		logger.From(ctx).Warn("chat: bad registration payload", slog.String("type", e.Type))
		return nil
	}
	if e.Type == events.RegistrationCreated && p.Status != "registered" {
		return nil // waitlisted players join when promoted
	}
	max := s.d.Settings.Int(ctx, "chat.group_max_members", 500)
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		g, err := lockEventGroup(ctx, tx, p.EventID, p.UserID)
		if db.IsNoRows(err) {
			return nil // no group (auto-create off)
		}
		if err != nil {
			return err
		}
		if g.ConvStatus != "active" || g.MemberStatus.String == "active" || g.MemberStatus.String == "banned" {
			return nil
		}
		if g.MemberCount >= max {
			logger.From(ctx).Warn("chat: event group full", slog.Int64("conversationId", g.ConversationID), slog.Int64("userId", p.UserID))
			return nil
		}
		if err := addMember(ctx, tx, g.ConversationID, p.UserID, "member"); err != nil {
			return err
		}
		return recountMembers(ctx, tx, g.ConversationID)
	})
}

// onRegistrationWithdrawn removes the player from the event group.
func (s *Service) onRegistrationWithdrawn(ctx context.Context, e events.Event) error {
	var p events.RegistrationPayload
	if err := e.Decode(&p); err != nil || p.EventID <= 0 || p.UserID <= 0 {
		logger.From(ctx).Warn("chat: bad registration payload", slog.String("type", e.Type))
		return nil
	}
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		g, err := lockEventGroup(ctx, tx, p.EventID, p.UserID)
		if db.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if g.MemberStatus.String != "active" {
			return nil
		}
		left, err := setMemberLeft(ctx, tx, g.ConversationID, p.UserID)
		if err != nil || !left {
			return err
		}
		return recountMembers(ctx, tx, g.ConversationID)
	})
}

// onEventClosed archives the group of a cancelled or completed event.
func (s *Service) onEventClosed(ctx context.Context, e events.Event) error {
	var p events.EventPayload
	if err := e.Decode(&p); err != nil || p.EventID <= 0 {
		logger.From(ctx).Warn("chat: bad event payload", slog.String("type", e.Type))
		return nil
	}
	return archiveEventGroup(ctx, s.d.DB, p.EventID)
}

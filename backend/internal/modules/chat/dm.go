package chat

import (
	"context"
	"database/sql"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/userdir"
)

var errCantMessage = httpx.Forbidden("You can't message this person.")

// CanMessage applies the DM rules of docs/api.md "Chat" (all must pass):
//  1. not blocked either way; recipient onboarding done and active;
//  2. recipient's privacy.allowDmsFrom (with chat.dm_requires_follow at least "followers");
//  3. if either is a minor, minor.dm_policy (none | mutual_follow | anyone);
//  4. for two adults an accepted consultation passes rule 2.
func (s *Service) CanMessage(ctx context.Context, sender userdir.Basic, recipient userdir.Basic) (bool, error) {
	if recipient.ID == sender.ID || recipient.Status != "active" || recipient.OnboardingStep != "done" {
		return false, nil
	}
	blocked, err := s.d.Users.BlockedEitherWay(ctx, sender.ID, recipient.ID)
	if err != nil || blocked {
		return false, err
	}

	anyMinor := sender.IsMinor || recipient.IsMinor
	if anyMinor {
		switch s.d.Settings.String(ctx, "minor.dm_policy", "mutual_follow") {
		case "anyone":
		case "mutual_follow":
			ok, err := s.d.Users.MutualFollow(ctx, sender.ID, recipient.ID)
			if err != nil || !ok {
				return false, err
			}
		default: // "none" and anything unknown: refuse (safest)
			return false, nil
		}
	}

	ok, err := s.privacyAllows(ctx, sender.ID, recipient.ID)
	if err != nil || ok {
		return ok, err
	}
	if anyMinor {
		return false, nil
	}
	return acceptedConsultation(ctx, s.d.DB, sender.ID, recipient.ID)
}

// privacyAllows is rule 2.
func (s *Service) privacyAllows(ctx context.Context, sender, recipient int64) (bool, error) {
	allow, err := allowDmsFrom(ctx, s.d.DB, recipient)
	if err != nil {
		return false, err
	}
	switch allow {
	case "followers", "mutual", "nobody":
	default:
		allow = "everyone"
	}
	if allow == "everyone" && s.d.Settings.Bool(ctx, "chat.dm_requires_follow", false) {
		allow = "followers"
	}
	switch allow {
	case "everyone":
		return true, nil
	case "followers":
		return s.d.Users.Follows(ctx, recipient, sender)
	case "mutual":
		return s.d.Users.MutualFollow(ctx, sender, recipient)
	}
	return false, nil
}

// Direct gets or creates the direct conversation between the caller and userID.
func (s *Service) Direct(ctx context.Context, me userdir.Basic, userID int64) (Conversation, error) {
	if userID <= 0 || userID == me.ID {
		return Conversation{}, httpx.Validation(map[string]string{"userId": "must be another user"})
	}
	other, err := s.d.Users.Get(ctx, userID)
	if db.IsNoRows(err) || (err == nil && other.Status == "deleted") {
		return Conversation{}, httpx.NotFound("User")
	}
	if err != nil {
		return Conversation{}, err
	}
	ok, err := s.CanMessage(ctx, me, other)
	if err != nil {
		return Conversation{}, err
	}
	if !ok {
		return Conversation{}, errCantMessage
	}
	key := directKey(me.ID, userID)
	id, err := directConversationID(ctx, s.d.DB, key)
	if db.IsNoRows(err) {
		err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
			var ierr error
			id, ierr = insertDirect(ctx, tx, key, me.ID, userID)
			return ierr
		})
		if db.IsDuplicate(err) { // created concurrently
			id, err = directConversationID(ctx, s.d.DB, key)
		}
	}
	if err != nil {
		return Conversation{}, err
	}
	return s.Conversation(ctx, me.ID, id)
}

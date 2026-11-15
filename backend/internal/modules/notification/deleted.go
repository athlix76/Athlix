package notification

import (
	"context"

	"athlix/backend/internal/platform/events"
)

// onUserDeleted removes every device token of a deleted account, so its phones get no
// more pushes (docs/security.md, account deletion: "tokens and devices removed").
// Idempotent: a second delivery deletes nothing. A malformed payload is dropped.
func (s *Service) onUserDeleted(ctx context.Context, e events.Event) error {
	var p events.UserPayload
	if err := e.Decode(&p); err != nil || p.UserID <= 0 {
		return nil
	}
	return s.repo.deleteUserDevices(ctx, p.UserID)
}

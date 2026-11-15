package subscription

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
)

const (
	defaultRestoreMax = 20
	maxRestoreCap     = 50
)

// Restore re-verifies every purchase the device's store SDK returned. A purchase that the store
// does not know, that belongs to another Athlix account or whose product is not a plan is
// counted in NotRestored; the call fails only when the store could not be reached for every
// purchase. Ownership is the same as verify: the purchase must carry the caller's
// obfuscated account token, so a restore never moves a purchase between accounts.
func (s *Service) Restore(ctx context.Context, userID int64, req restoreRequest) (RestoreResult, error) {
	limit := s.d.Settings.Int(ctx, "premium.restore_max_purchases", defaultRestoreMax)
	if limit < 1 || limit > maxRestoreCap {
		limit = defaultRestoreMax
	}
	if len(req.Purchases) == 0 || len(req.Purchases) > limit {
		return RestoreResult{}, httpx.Validation(map[string]string{"purchases": "must have 1 to " + strconv.Itoa(limit) + " items"})
	}
	var out RestoreResult
	var upstream error
	for _, p := range req.Purchases {
		_, err := s.Verify(ctx, userID, p)
		if err == nil {
			out.Restored++
			continue
		}
		out.NotRestored++
		if e, ok := httpx.As(err); ok && e.Status >= http.StatusInternalServerError {
			upstream = err
		} else if !ok {
			return RestoreResult{}, err
		}
	}
	if out.Restored == 0 && upstream != nil && out.NotRestored == len(req.Purchases) {
		return RestoreResult{}, upstream
	}
	logger.From(ctx).Info("premium restore", slog.Int64("userId", userID), slog.Int("restored", out.Restored), slog.Int("notRestored", out.NotRestored))
	mine, err := s.Mine(ctx, userID)
	if err != nil {
		return RestoreResult{}, err
	}
	out.Subscription = mine
	return out, nil
}

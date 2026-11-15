package community

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
)

// AdminList lists communities of any status (?q&status), newest first.
func (s *Service) AdminList(ctx context.Context, q, status string, p httpx.PageParams) ([]AdminCommunity, string, error) {
	switch status {
	case "", StatusActive, StatusArchived, StatusRemoved:
	default:
		return nil, "", httpx.Validation(map[string]string{"status": "must be active, archived or removed"})
	}
	rows, err := s.r.adminList(ctx, s.d.DB, strings.TrimSpace(q), status, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	cs, err := s.toCommunities(ctx, rows)
	if err != nil {
		return nil, "", err
	}
	out := make([]AdminCommunity, len(cs))
	for i, c := range cs {
		out[i] = AdminCommunity{Community: c, Status: rows[i].Status, RemovalReason: nullStr(rows[i].Removal)}
	}
	return out, next, nil
}

// AdminRemove sets status 'removed', stores the reason and writes the audit row in the same tx. Removing an
// already removed community is a no-op (204, no second audit row).
func (s *Service) AdminRemove(c *gin.Context, id int64, in RemoveInput) error {
	ctx := c.Request.Context()
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return httpx.Validation(map[string]string{"reason": "is required"})
	}
	if utf8.RuneCountInString(reason) > reasonMax {
		return httpx.Validation(map[string]string{"reason": "must be at most 500 characters"})
	}
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var prev string
		err := tx.QueryRowContext(ctx, `SELECT status FROM conversations WHERE id = ? AND type = 'community' FOR UPDATE`, id).Scan(&prev)
		if db.IsNoRows(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if prev == StatusRemoved {
			return nil
		}
		// The reason is stored on the row (conversations.removal_reason) for the admin panel;
		// it is never copied into logs or the audit details.
		if err := s.r.remove(ctx, tx, id, reason); err != nil {
			return err
		}
		logger.From(ctx).Info("community removed by admin", slog.Int64("communityId", id))
		// details hold ids / flags only (audit rule), so the free-text reason is not copied.
		return audit.Admin(c, tx, "community.remove", "conversation", itoa(id),
			map[string]any{"previousStatus": prev, "reasonLength": utf8.RuneCountInString(reason)})
	})
}

package chat

import (
	"database/sql"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/httpx"
)

// maxReadReasonRunes bounds the optional ?reason= on the admin read.
const maxReadReasonRunes = 255

// adminMessages is GET /admin/conversations/:id/messages (perm moderation.read_messages).
// Only for a conversation with an open report on it or on one of its messages; otherwise 404.
// Every read is written to admin_audit_log in the same transaction as the report check.
func (h *handler) adminMessages(c *gin.Context) {
	ctx := c.Request.Context()
	convID, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errConvNotFound)
		return
	}
	before, err := httpx.QueryInt64(c, "before")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	p, err := httpx.ParsePage(c, defaultMessagesLimit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if before == 0 {
		before = p.Cursor.ID
	}
	// The admin states why they read the messages; it goes into the audit row (never logged).
	reason := strings.TrimSpace(c.Query("reason"))
	if utf8.RuneCountInString(reason) > maxReadReasonRunes {
		httpx.Fail(c, httpx.Validation(map[string]string{"reason": "must be at most 255 characters"}))
		return
	}
	details := map[string]any{"before": before, "limit": p.Limit}
	if reason != "" {
		details["reason"] = reason
	}
	err = db.WithTx(ctx, h.s.d.DB, func(tx *sql.Tx) error {
		ok, err := openReportExists(ctx, tx, convID)
		if err != nil {
			return err
		}
		if !ok {
			return errConvNotFound
		}
		return audit.Admin(c, tx, "conversation.read_messages", "conversation", strconv.FormatInt(convID, 10),
			details)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	msgs, next, err := h.s.messagesPage(ctx, convID, before, p.Limit, true)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, msgs, httpx.Meta{NextCursor: next})
}

package notification

import (
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ s *Service }

// POST /v1/notifications/devices (user)
func (h *handler) registerDevice(c *gin.Context) {
	var in deviceInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.s.RegisterDevice(c.Request.Context(), authn.UserID(c), in); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// DELETE /v1/notifications/devices (user)
func (h *handler) deleteDevice(c *gin.Context) {
	var in deviceDeleteInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.s.DeleteDevice(c.Request.Context(), authn.UserID(c), in.Token); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// GET /v1/notifications (verified)
func (h *handler) list(c *gin.Context) {
	p, err := httpx.ParsePage(c, listDefaultLimit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, unread, err := h.s.List(c.Request.Context(), authn.UserID(c), p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next, UnreadCount: &unread})
}

// POST /v1/notifications/read (verified)
func (h *handler) read(c *gin.Context) {
	var in readInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.s.MarkRead(c.Request.Context(), authn.UserID(c), in); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// POST /v1/admin/notifications/broadcast (notifications.broadcast)
func (h *handler) broadcast(c *gin.Context) {
	var in BroadcastInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	res, err := h.s.Broadcast(c, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}

// GET /v1/notifications/unread-count (verified)
func (h *handler) unreadCount(c *gin.Context) {
	n, err := h.s.UnreadCount(c.Request.Context(), authn.UserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"unreadCount": n})
}

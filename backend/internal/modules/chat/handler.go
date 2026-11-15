package chat

import (
	"time"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

// handler adapts HTTP to the service. The caller always comes from the verified token.
type handler struct{ s *Service }

func (h *handler) wsTicket(c *gin.Context) {
	t, err := h.s.IssueTicket(c.Request.Context(), authn.UserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, t)
}

func (h *handler) conversations(c *gin.Context) {
	p, err := httpx.ParsePage(c, defaultConvLimit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.s.Conversations(c.Request.Context(), authn.UserID(c), p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) direct(c *gin.Context) {
	var in struct {
		UserID int64 `json:"userId"`
	}
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	conv, err := h.s.Direct(c.Request.Context(), authn.User(c), in.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, conv)
}

// convID reads :id; a bad id is 404 like a conversation the caller can't see.
func convID(c *gin.Context) (int64, bool) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errConvNotFound)
		return 0, false
	}
	return id, true
}

func (h *handler) conversation(c *gin.Context) {
	id, ok := convID(c)
	if !ok {
		return
	}
	conv, err := h.s.Conversation(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, conv)
}

func (h *handler) messages(c *gin.Context) {
	id, ok := convID(c)
	if !ok {
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
	items, next, err := h.s.Messages(c.Request.Context(), authn.UserID(c), id, before, p.Limit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) send(c *gin.Context) {
	id, ok := convID(c)
	if !ok {
		return
	}
	var in SendInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	msg, _, err := h.s.SendMessage(c.Request.Context(), authn.UserID(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, msg)
}

func (h *handler) deleteMessage(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errMessageNotFound)
		return
	}
	if err := h.s.DeleteMessage(c.Request.Context(), authn.UserID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) read(c *gin.Context) {
	id, ok := convID(c)
	if !ok {
		return
	}
	var in struct {
		MessageID int64 `json:"messageId"`
	}
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.s.MarkRead(c.Request.Context(), authn.UserID(c), id, in.MessageID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) mute(c *gin.Context) {
	id, ok := convID(c)
	if !ok {
		return
	}
	var in struct {
		MutedUntil *time.Time `json:"mutedUntil"`
	}
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.s.Mute(c.Request.Context(), authn.UserID(c), id, in.MutedUntil); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) leave(c *gin.Context) {
	id, ok := convID(c)
	if !ok {
		return
	}
	if err := h.s.Leave(c.Request.Context(), authn.UserID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) members(c *gin.Context) {
	id, ok := convID(c)
	if !ok {
		return
	}
	p, err := httpx.ParsePage(c, defaultMembersLimit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.s.Members(c.Request.Context(), authn.UserID(c), id, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) eventGroup(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errConvNotFound)
		return
	}
	conv, err := h.s.EventGroup(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, conv)
}

func (h *handler) unreadCount(c *gin.Context) {
	out, err := h.s.UnreadCounts(c.Request.Context(), authn.UserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

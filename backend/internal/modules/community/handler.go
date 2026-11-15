package community

import (
	"strings"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

// handler adapts HTTP to the service. The caller always comes from the verified token.
type handler struct{ svc *Service }

func (h *handler) id(c *gin.Context) (int64, bool) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, errNotFound)
		return 0, false
	}
	return id, true
}

func (h *handler) ids(c *gin.Context) (int64, int64, bool) {
	id, ok := h.id(c)
	if !ok {
		return 0, 0, false
	}
	uid, err := httpx.PathID(c, "userId")
	if err != nil {
		httpx.Fail(c, httpx.NotFound("member"))
		return 0, 0, false
	}
	return id, uid, true
}

func (h *handler) create(c *gin.Context) {
	var in Input
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Create(c.Request.Context(), authn.User(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) browse(c *gin.Context) {
	p, err := httpx.ParsePage(c, defaultPage)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f := ListFilter{Q: strings.TrimSpace(c.Query("q")), State: strings.TrimSpace(c.Query("state"))}
	if f.SportID, err = httpx.QueryInt64(c, "sportId"); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, next, err := h.svc.Browse(c.Request.Context(), authn.UserID(c), f, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, out, httpx.Meta{NextCursor: next})
}

func (h *handler) mine(c *gin.Context) {
	p, err := httpx.ParsePage(c, defaultPage)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, next, err := h.svc.Mine(c.Request.Context(), authn.UserID(c), p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, out, httpx.Meta{NextCursor: next})
}

func (h *handler) get(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) update(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	var in Input
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Update(c.Request.Context(), authn.UserID(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) remove(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), authn.UserID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) join(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	out, err := h.svc.Join(c.Request.Context(), authn.User(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) joinRequests(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	p, err := httpx.ParsePage(c, defaultPage)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, next, err := h.svc.JoinRequests(c.Request.Context(), authn.UserID(c), id, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, out, httpx.Meta{NextCursor: next})
}

func (h *handler) approve(c *gin.Context) {
	id, uid, ok := h.ids(c)
	if !ok {
		return
	}
	if err := h.svc.Approve(c.Request.Context(), authn.UserID(c), id, uid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) reject(c *gin.Context) {
	id, uid, ok := h.ids(c)
	if !ok {
		return
	}
	if err := h.svc.Reject(c.Request.Context(), authn.UserID(c), id, uid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) invite(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	var in InviteInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Invite(c.Request.Context(), authn.User(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) accept(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	out, err := h.svc.AcceptInvite(c.Request.Context(), authn.User(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) removeMember(c *gin.Context) {
	id, uid, ok := h.ids(c)
	if !ok {
		return
	}
	if err := h.svc.RemoveMember(c.Request.Context(), authn.UserID(c), id, uid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) banMember(c *gin.Context) {
	id, uid, ok := h.ids(c)
	if !ok {
		return
	}
	if err := h.svc.BanMember(c.Request.Context(), authn.UserID(c), id, uid); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) setRole(c *gin.Context) {
	id, uid, ok := h.ids(c)
	if !ok {
		return
	}
	var in RoleInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.SetRole(c.Request.Context(), authn.UserID(c), id, uid, in); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) transferOwnership(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	var in TransferInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.TransferOwnership(c.Request.Context(), authn.UserID(c), id, in); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) adminList(c *gin.Context) {
	p, err := httpx.ParsePage(c, defaultPage)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, next, err := h.svc.AdminList(c.Request.Context(), c.Query("q"), c.Query("status"), p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, out, httpx.Meta{NextCursor: next})
}

func (h *handler) adminRemove(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	var in RemoveInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.AdminRemove(c, id, in); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

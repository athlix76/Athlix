package follow

import (
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/userdir"
)

type handler struct{ svc *Service }

// target reads :userId; the caller always comes from the token.
func target(c *gin.Context) (int64, bool) {
	id, err := httpx.PathID(c, "userId")
	if err != nil {
		httpx.Fail(c, err)
		return 0, false
	}
	return id, true
}

func (h *handler) state(c *gin.Context, fn func(*gin.Context, userdir.Basic, int64) (State, error)) {
	id, ok := target(c)
	if !ok {
		return
	}
	st, err := fn(c, authn.User(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, st)
}

func (h *handler) follow(c *gin.Context) {
	h.state(c, func(c *gin.Context, me userdir.Basic, id int64) (State, error) {
		return h.svc.Follow(c.Request.Context(), me, id)
	})
}

func (h *handler) unfollow(c *gin.Context) {
	h.state(c, func(c *gin.Context, me userdir.Basic, id int64) (State, error) {
		return h.svc.Unfollow(c.Request.Context(), me, id)
	})
}

func (h *handler) block(c *gin.Context) {
	id, ok := target(c)
	if !ok {
		return
	}
	if err := h.svc.Block(c.Request.Context(), authn.User(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) unblock(c *gin.Context) {
	id, ok := target(c)
	if !ok {
		return
	}
	if err := h.svc.Unblock(c.Request.Context(), authn.User(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

type listFn func(c *gin.Context, me userdir.Basic, page httpx.PageParams) ([]userdir.Summary, string, error)

func (h *handler) page(c *gin.Context, fn listFn) {
	page, err := httpx.ParsePage(c, 20)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := fn(c, authn.User(c), page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) followers(c *gin.Context) {
	id, ok := target(c)
	if !ok {
		return
	}
	h.page(c, func(c *gin.Context, me userdir.Basic, p httpx.PageParams) ([]userdir.Summary, string, error) {
		return h.svc.Followers(c.Request.Context(), me, id, p)
	})
}

func (h *handler) following(c *gin.Context) {
	id, ok := target(c)
	if !ok {
		return
	}
	h.page(c, func(c *gin.Context, me userdir.Basic, p httpx.PageParams) ([]userdir.Summary, string, error) {
		return h.svc.Following(c.Request.Context(), me, id, p)
	})
}

func (h *handler) blocks(c *gin.Context) {
	h.page(c, func(c *gin.Context, me userdir.Basic, p httpx.PageParams) ([]userdir.Summary, string, error) {
		return h.svc.Blocks(c.Request.Context(), me, p)
	})
}

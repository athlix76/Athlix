package profile

import (
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) setup(c *gin.Context) {
	var in setupBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	me, err := h.svc.Setup(c.Request.Context(), authn.User(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, me)
}

// target reads :userId; the caller always comes from the token.
func target(c *gin.Context) (int64, bool) {
	id, err := httpx.PathID(c, "userId")
	if err != nil {
		httpx.Fail(c, err)
		return 0, false
	}
	return id, true
}

func (h *handler) view(c *gin.Context) {
	id, ok := target(c)
	if !ok {
		return
	}
	p, err := h.svc.View(c.Request.Context(), authn.User(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, p)
}

func (h *handler) update(c *gin.Context) {
	var in updateBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	p, err := h.svc.Update(c.Request.Context(), authn.User(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, p)
}

func (h *handler) viewers(c *gin.Context) {
	page, err := httpx.ParsePage(c, 20)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.svc.Viewers(c.Request.Context(), authn.User(c), page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) stats(c *gin.Context) {
	id, ok := target(c)
	if !ok {
		return
	}
	out, err := h.svc.Stats(c.Request.Context(), authn.User(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) createStat(c *gin.Context) {
	var in statCreateBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreateStat(c.Request.Context(), authn.User(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) updateStat(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in statUpdateBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.UpdateStat(c.Request.Context(), authn.User(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) deleteStat(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteStat(c.Request.Context(), authn.User(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) achievements(c *gin.Context) {
	id, ok := target(c)
	if !ok {
		return
	}
	out, err := h.svc.Achievements(c.Request.Context(), authn.User(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) createAchievement(c *gin.Context) {
	var in achievementBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreateAchievement(c.Request.Context(), authn.User(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) updateAchievement(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in achievementBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.UpdateAchievement(c.Request.Context(), authn.User(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) deleteAchievement(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteAchievement(c.Request.Context(), authn.User(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

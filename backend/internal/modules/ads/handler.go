package ads

import (
	"strings"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) serve(c *gin.Context) {
	sportID, err := httpx.QueryInt64(c, "sportId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Serve(c.Request.Context(), authn.User(c), c.Query("placement"), sportID, c.Query("state"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) track(click bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.PathID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := h.svc.Track(c.Request.Context(), authn.User(c), id, click); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.NoContent(c)
	}
}

func (h *handler) adminList(c *gin.Context) {
	p, err := httpx.ParsePage(c, 20)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, next, err := h.svc.List(c.Request.Context(), strings.TrimSpace(c.Query("status")), p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, rows, httpx.Meta{NextCursor: next})
}

func (h *handler) adminCreate(c *gin.Context) {
	var in CampaignInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Create(c, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) adminUpdate(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in CampaignInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Update(c, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) adminStats(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Stats(c.Request.Context(), id, strings.TrimSpace(c.Query("from")), strings.TrimSpace(c.Query("to")))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

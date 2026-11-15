package registration

import (
	"strings"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) register(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in RegisterInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Register(c.Request.Context(), authn.UserID(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *handler) withdraw(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.Withdraw(c.Request.Context(), authn.UserID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) mine(c *gin.Context) {
	pg, err := httpx.ParsePage(c, defPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.svc.Mine(c.Request.Context(), authn.UserID(c), strings.TrimSpace(c.Query("status")), pg)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) forEvent(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	pg, err := httpx.ParsePage(c, defPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.svc.ForEvent(c.Request.Context(), authn.UserID(c), id, strings.TrimSpace(c.Query("status")),
		httpx.QueryBool(c, "flagged"), pg)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) adminForEvent(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	pg, err := httpx.ParsePage(c, defPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.svc.AdminForEvent(c.Request.Context(), id, strings.TrimSpace(c.Query("status")),
		httpx.QueryBool(c, "flagged"), pg)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) patch(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	regID, err := httpx.PathID(c, "regId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in PatchInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Patch(c.Request.Context(), authn.UserID(c), id, regID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

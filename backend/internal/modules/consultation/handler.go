package consultation

import (
	"context"

	"strings"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) getListing(c *gin.Context) {
	coachID, err := httpx.PathID(c, "userId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.GetListing(c.Request.Context(), authn.UserID(c), coachID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) myListing(c *gin.Context) {
	out, err := h.svc.MyListing(c.Request.Context(), authn.User(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) putListing(c *gin.Context) {
	var in ListingInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.PutListing(c.Request.Context(), authn.User(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) createRequest(c *gin.Context) {
	coachID, err := httpx.PathID(c, "userId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in RequestInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreateRequest(c.Request.Context(), authn.UserID(c), coachID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) listRequests(c *gin.Context) {
	p, err := httpx.ParsePage(c, 20)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	as := strings.TrimSpace(c.Query("as"))
	if as == "" {
		as = "player"
	}
	rows, next, err := h.svc.List(c.Request.Context(), authn.UserID(c), as, strings.TrimSpace(c.Query("status")), p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, rows, httpx.Meta{NextCursor: next})
}

func (h *handler) respond(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in RespondInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Respond(c.Request.Context(), authn.UserID(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) cancel(c *gin.Context) {
	h.simple(c, h.svc.Cancel)
}

func (h *handler) complete(c *gin.Context) {
	h.simple(c, h.svc.Complete)
}

func (h *handler) simple(c *gin.Context, fn func(ctx context.Context, userID, id int64) (Request, error)) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := fn(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

package subscription

import (
	"io"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

// maxWebhookBody bounds store notification bodies (the router also caps bodies at 1 MiB).
const maxWebhookBody = 256 << 10

func (h *handler) plans(c *gin.Context) {
	out, err := h.svc.Plans(c.Request.Context(), c.Query("audience"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) mine(c *gin.Context) {
	out, err := h.svc.Mine(c.Request.Context(), authn.UserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) verify(c *gin.Context) {
	var req verifyRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Verify(c.Request.Context(), authn.UserID(c), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) restore(c *gin.Context) {
	var req restoreRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Restore(c.Request.Context(), authn.UserID(c), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) mockActivate(c *gin.Context) {
	var req mockActivateRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.MockActivate(c.Request.Context(), authn.UserID(c), req.PlanID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func webhookOK(c *gin.Context) { httpx.OK(c, gin.H{"received": true}) }

// googlePlay: the Authorization header is verified before the body is read.
func (h *handler) googlePlay(c *gin.Context) {
	read := func() ([]byte, error) { return io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBody)) }
	if err := h.svc.HandlePlayPush(c.Request.Context(), c.GetHeader("Authorization"), read); err != nil {
		httpx.Fail(c, err)
		return
	}
	webhookOK(c)
}

func (h *handler) appStore(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBody))
	if err != nil {
		httpx.Fail(c, errWebhookBody())
		return
	}
	if err := h.svc.HandleAppleNotification(c.Request.Context(), body); err != nil {
		httpx.Fail(c, err)
		return
	}
	webhookOK(c)
}

// ---- admin ----

func (h *handler) adminPlans(c *gin.Context) {
	out, err := h.svc.AdminPlans(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) createPlan(c *gin.Context) {
	var in planInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreatePlan(c, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *handler) updatePlan(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in planInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.UpdatePlan(c, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) replaceBenefits(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in []benefitInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.ReplaceBenefits(c, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) adminSubscriptions(c *gin.Context) {
	page, err := httpx.ParsePage(c, defaultPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var f adminSubFilter
	if f.UserID, err = httpx.QueryInt64(c, "userId"); err != nil {
		httpx.Fail(c, err)
		return
	}
	if f.PlanID, err = httpx.QueryInt64(c, "planId"); err != nil {
		httpx.Fail(c, err)
		return
	}
	f.Status = c.Query("status")
	items, meta, err := h.svc.AdminSubscriptions(c.Request.Context(), f, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, meta)
}

func (h *handler) grant(c *gin.Context) {
	var req grantRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Grant(c, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

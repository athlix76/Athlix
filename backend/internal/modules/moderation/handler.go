package moderation

import (
	"strings"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) createReport(c *gin.Context) {
	var in CreateReportInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	ref, created, err := h.svc.CreateReport(c.Request.Context(), authn.UserID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if created {
		httpx.Created(c, ref)
		return
	}
	httpx.OK(c, ref)
}

func (h *handler) listReports(c *gin.Context) {
	p, err := httpx.ParsePage(c, defaultPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f := ReportFilter{
		Status:     strings.TrimSpace(c.Query("status")),
		TargetType: strings.TrimSpace(c.Query("targetType")),
		Reason:     strings.TrimSpace(c.Query("reason")),
	}
	items, next, err := h.svc.ListReports(c, f, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) resolve(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, httpx.NotFound("Report"))
		return
	}
	var in ResolveInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Resolve(c, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) listActions(c *gin.Context) {
	p, err := httpx.ParsePage(c, defaultPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	targetID, err := httpx.QueryInt64(c, "targetId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.svc.ListActions(c.Request.Context(), strings.TrimSpace(c.Query("targetType")), targetID, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) createAction(c *gin.Context) {
	var in ActionInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreateAction(c, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

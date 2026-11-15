package locationidentity

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

// adminAudit binds audit.Admin to this request (admin id + IP come from the gin context).
func adminAudit(c *gin.Context) auditFn {
	return func(q db.Querier, action, targetType string, targetID int64, details any) error {
		return audit.Admin(c, q, action, targetType, strconv.FormatInt(targetID, 10), details)
	}
}

// config writes the config_audit row for a districts change.
func (auditFn) config(ctx context.Context, q db.Querier, id int64, old, new any, adminID int64) error {
	return audit.Config(ctx, q, "districts", strconv.FormatInt(id, 10), old, new, adminID)
}

// ---- user ----

func (h *handler) states(c *gin.Context) {
	out, err := h.svc.States(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) districts(c *gin.Context) {
	stateID, err := httpx.QueryInt64(c, "stateId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Districts(c.Request.Context(), stateID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) mine(c *gin.Context) {
	out, err := h.svc.Mine(c.Request.Context(), authn.User(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) submit(c *gin.Context) {
	var in LocationIdentityInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Submit(c.Request.Context(), authn.User(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *handler) withdraw(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, httpx.NotFound("Location identity"))
		return
	}
	if err := h.svc.Withdraw(c.Request.Context(), authn.UserID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// ---- admin ----

func (h *handler) queue(c *gin.Context) {
	p, err := httpx.ParsePage(c, adminPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f := QueueFilter{Status: strings.TrimSpace(c.Query("status")), IdentityType: strings.TrimSpace(c.Query("identityType"))}
	if f.StateID, err = httpx.QueryInt64(c, "stateId"); err != nil {
		httpx.Fail(c, err)
		return
	}
	if f.DistrictID, err = httpx.QueryInt64(c, "districtId"); err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.svc.Queue(c.Request.Context(), f, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) detail(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, httpx.NotFound("Location identity"))
		return
	}
	out, err := h.svc.Detail(c.Request.Context(), id, true, adminAudit(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) approve(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, httpx.NotFound("Location identity"))
		return
	}
	var in approveBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Approve(c.Request.Context(), authn.AdminID(c), id, in, adminAudit(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) reject(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, httpx.NotFound("Location identity"))
		return
	}
	var in rejectBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Reject(c.Request.Context(), authn.AdminID(c), id, in, adminAudit(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) revoke(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, httpx.NotFound("Location identity"))
		return
	}
	var in revokeBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Revoke(c.Request.Context(), id, in, adminAudit(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) adminStates(c *gin.Context) {
	out, err := h.svc.AdminStates(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) adminDistricts(c *gin.Context) {
	p, err := httpx.ParsePage(c, adminPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f := DistrictFilter{Q: c.Query("q"), IncludeInactive: httpx.QueryBool(c, "includeInactive")}
	if f.StateID, err = httpx.QueryInt64(c, "stateId"); err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.svc.AdminDistricts(c.Request.Context(), f, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) createDistrict(c *gin.Context) {
	var in districtCreateBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreateDistrict(c.Request.Context(), authn.AdminID(c), in, adminAudit(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *handler) updateDistrict(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, httpx.NotFound("District"))
		return
	}
	var in districtUpdateBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.UpdateDistrict(c.Request.Context(), authn.AdminID(c), id, in, adminAudit(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

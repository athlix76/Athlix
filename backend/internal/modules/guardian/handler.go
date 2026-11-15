package guardian

import (
	"database/sql"
	"strconv"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/audit"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) request(c *gin.Context) {
	var in requestBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	t, err := h.svc.Request(c.Request.Context(), authn.UserID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, t)
}

func (h *handler) verify(c *gin.Context) {
	var in verifyBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	me, err := h.svc.Verify(c.Request.Context(), authn.UserID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, me)
}

func (h *handler) withdrawRequest(c *gin.Context) {
	t, err := h.svc.WithdrawRequest(c.Request.Context(), authn.UserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, t)
}

func (h *handler) withdrawVerify(c *gin.Context) {
	var in withdrawVerifyBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	me, err := h.svc.WithdrawVerify(c.Request.Context(), authn.UserID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, me)
}

func (h *handler) adminRevoke(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in adminRevokeBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.AdminRevoke(c.Request.Context(), id, in, func(tx *sql.Tx) error {
		return audit.Admin(c, tx, "guardian.consent_revoke", "user", strconv.FormatInt(id, 10), nil)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

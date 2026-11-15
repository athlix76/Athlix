package user

import (
	"database/sql"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) getMe(c *gin.Context) {
	m, err := h.svc.me(c.Request.Context(), authn.UserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, m)
}

func (h *handler) deleteMe(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), authn.UserID(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) getSettings(c *gin.Context) {
	s, err := h.svc.Settings(c.Request.Context(), authn.UserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, s)
}

func (h *handler) putSettings(c *gin.Context) {
	var in settingsUpdate
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	s, err := h.svc.UpdateSettings(c.Request.Context(), authn.UserID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, s)
}

func (h *handler) acceptPolicies(c *gin.Context) {
	var in acceptPoliciesRequest
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	m, err := h.svc.AcceptPolicies(c.Request.Context(), authn.UserID(c), in.PolicyDocumentIDs)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, m)
}

func (h *handler) addRole(c *gin.Context) {
	var in addRoleRequest
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	m, err := h.svc.AddRole(c.Request.Context(), authn.UserID(c), in.Role)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, m)
}

func (h *handler) chooseRole(c *gin.Context) {
	var in onboardingRoleRequest
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	m, err := h.svc.ChooseRole(c.Request.Context(), authn.UserID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, m)
}

func (h *handler) adminList(c *gin.Context) {
	page, err := httpx.ParsePage(c, 20)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f, err := h.svc.parseAdminFilter(c.Query("q"), c.Query("role"), c.Query("status"), c.Query("verificationStatus"), c.Query("isMinor"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f.BeforeID, f.Limit = page.Cursor.ID, page.Limit
	rows, next, err := h.svc.AdminList(c.Request.Context(), f)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, rows, httpx.Meta{NextCursor: next})
}

func (h *handler) adminDetail(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.AdminDetail(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := audit.Admin(c, h.svc.d.DB, "user.view", "user", strconv.FormatInt(id, 10), nil); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) adminSuspend(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in suspendRequest
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > 255 {
		httpx.Fail(c, httpx.Validation(map[string]string{"reason": "is required (at most 255 characters)"}))
		return
	}
	row, err := h.svc.changeStatus(c.Request.Context(), id, true, reason, func(tx *sql.Tx) error {
		return audit.Admin(c, tx, "user.suspend", "user", strconv.FormatInt(id, 10), nil)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, row)
}

func (h *handler) adminUnsuspend(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	row, err := h.svc.changeStatus(c.Request.Context(), id, false, "", func(tx *sql.Tx) error {
		return audit.Admin(c, tx, "user.unsuspend", "user", strconv.FormatInt(id, 10), nil)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, row)
}

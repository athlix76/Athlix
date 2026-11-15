package admin

import (
	"database/sql"
	"strconv"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) login(c *gin.Context) {
	var in loginBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Login(c.Request.Context(), in, ClientInfo{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) refresh(c *gin.Context) {
	var in refreshBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	p, err := h.svc.Refresh(c.Request.Context(), in.RefreshToken)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, p)
}

func (h *handler) logout(c *gin.Context) {
	var in refreshBody
	if c.Request.ContentLength != 0 {
		if err := httpx.BindJSON(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	if err := h.svc.Logout(c.Request.Context(), authn.AdminID(c), in.RefreshToken); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *handler) me(c *gin.Context) {
	a, err := h.svc.Me(c.Request.Context(), authn.AdminID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, a)
}

func (h *handler) dashboard(c *gin.Context) {
	d, err := h.svc.Dashboard(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, d)
}

func (h *handler) listAdmins(c *gin.Context) {
	out, err := h.svc.ListAdmins(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) createAdmin(c *gin.Context) {
	var in createAdminBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreateViaAPI(c.Request.Context(), in, func(tx *sql.Tx, id int64) error {
		return audit.Admin(c, tx, "admin.create", "admin", strconv.FormatInt(id, 10), map[string]any{"roleId": in.RoleID})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *handler) updateAdmin(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in updateAdminBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Update(c.Request.Context(), authn.AdminID(c), id, in, func(tx *sql.Tx, details map[string]any) error {
		return audit.Admin(c, tx, "admin.update", "admin", strconv.FormatInt(id, 10), details)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) resetCredentials(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.ResetCredentials(c.Request.Context(), id, func(tx *sql.Tx) error {
		return audit.Admin(c, tx, "admin.reset_credentials", "admin", strconv.FormatInt(id, 10), nil)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) listRoles(c *gin.Context) {
	out, err := h.svc.Roles(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) createRole(c *gin.Context) {
	var in createRoleBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreateRole(c.Request.Context(), in, func(tx *sql.Tx, id int64) error {
		return audit.Admin(c, tx, "role.create", "admin_role", strconv.FormatInt(id, 10), nil)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *handler) updateRole(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in updateRoleBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.UpdateRole(c.Request.Context(), id, in, func(tx *sql.Tx, details map[string]any) error {
		return audit.Admin(c, tx, "role.update", "admin_role", strconv.FormatInt(id, 10), details)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) auditLog(c *gin.Context) {
	p, err := httpx.ParsePage(c, 50)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f, err := h.svc.parseAuditFilter(c.Query("adminId"), c.Query("action"), c.Query("targetType"), c.Query("targetId"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f.BeforeID, f.Limit = p.Cursor.ID, p.Limit
	rows, next, err := h.svc.AuditLog(c.Request.Context(), f)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, rows, httpx.Meta{NextCursor: next})
}

func (h *handler) changePassword(c *gin.Context) {
	var in changePasswordBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	id := authn.AdminID(c)
	out, err := h.svc.ChangePassword(c.Request.Context(), id, in, func(tx *sql.Tx) error {
		return audit.Admin(c, tx, "admin.change_password", "admin", strconv.FormatInt(id, 10), nil)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) loginEvents(c *gin.Context) {
	p, err := httpx.ParsePage(c, 50)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f, err := h.svc.parseLoginEventFilter(c.Query("adminId"), c.Query("email"), c.Query("outcome"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f.BeforeID, f.Limit = p.Cursor.ID, p.Limit
	rows, next, err := h.svc.LoginEvents(c.Request.Context(), f)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, rows, httpx.Meta{NextCursor: next})
}

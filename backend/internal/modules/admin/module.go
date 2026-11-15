package admin

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
)

// Module is the admin module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module { return &Module{d: d, svc: newService(d)} }

// Name implements app.Module.
func (m *Module) Name() string { return "admin" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := &handler{svc: m.svc}
	r.AdminPublic.POST("/auth/login", h.login)
	r.AdminPublic.POST("/auth/refresh", h.refresh)

	r.Admin.POST("/auth/logout", h.logout)
	r.Admin.GET("/me", h.me)
	r.Admin.POST("/me/password", h.changePassword)
	r.Admin.GET("/dashboard", authn.RequirePermission("dashboard.read"), h.dashboard)

	manage := authn.RequirePermission("admins.manage")
	r.Admin.GET("/admins", manage, h.listAdmins)
	r.Admin.POST("/admins", manage, h.createAdmin)
	r.Admin.PUT("/admins/:id", manage, h.updateAdmin)
	r.Admin.POST("/admins/:id/reset-credentials", manage, h.resetCredentials)
	r.Admin.GET("/roles", manage, h.listRoles)
	r.Admin.POST("/roles", manage, h.createRole)
	r.Admin.PUT("/roles/:id", manage, h.updateRole)

	r.Admin.GET("/audit-log", authn.RequirePermission("audit.read"), h.auditLog)
	r.Admin.GET("/login-events", authn.RequirePermission("audit.read"), h.loginEvents)
}

// Jobs implements app.JobProvider.
func (m *Module) Jobs() map[string]app.JobFunc {
	return map[string]app.JobFunc{"admin.purge_login_events": m.svc.purgeLoginEvents}
}

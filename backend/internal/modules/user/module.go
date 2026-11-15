package user

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
)

// Module is the user module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module { return &Module{d: d, svc: newService(d)} }

// Name implements app.Module.
func (m *Module) Name() string { return "user" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := &handler{svc: m.svc}
	r.User.GET("/me", h.getMe)
	r.User.DELETE("/me", h.deleteMe)
	r.User.GET("/me/settings", h.getSettings)
	r.User.PUT("/me/settings", h.putSettings)
	r.User.POST("/me/policies/accept", h.acceptPolicies)
	r.User.POST("/onboarding/role", h.chooseRole)
	r.Verified.POST("/me/roles", h.addRole)

	r.Admin.GET("/users", authn.RequirePermission("users.read"), h.adminList)
	r.Admin.GET("/users/:id", authn.RequirePermission("users.read"), h.adminDetail)
	r.Admin.POST("/users/:id/suspend", authn.RequirePermission("users.write"), h.adminSuspend)
	r.Admin.POST("/users/:id/unsuspend", authn.RequirePermission("users.write"), h.adminUnsuspend)
}

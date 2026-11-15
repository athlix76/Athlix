package guardian

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/events"
)

// Module is the guardian module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module { return &Module{d: d, svc: newService(d)} }

// Name implements app.Module.
func (m *Module) Name() string { return "guardian" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := &handler{svc: m.svc}
	r.User.POST("/onboarding/guardian/request", h.request)
	r.User.POST("/onboarding/guardian/verify", h.verify)
	r.User.POST("/me/guardian/withdraw/request", h.withdrawRequest)
	r.User.POST("/me/guardian/withdraw/verify", h.withdrawVerify)
	r.Admin.POST("/users/:id/guardian-consent/revoke", authn.RequirePermission("users.write"), h.adminRevoke)
}

// EventHandlers implements the optional app interface.
func (m *Module) EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{events.UserDeleted: m.svc.onUserDeleted}
}

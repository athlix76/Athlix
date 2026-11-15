package organiser

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/events"
)

// Module is the organiser module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module { return &Module{d: d, svc: newService(d)} }

// Name implements app.Module.
func (m *Module) Name() string { return "organiser" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := &handler{svc: m.svc}
	org := r.Verified.Group("/organiser/verification", requireOrganiser)
	org.GET("", h.get)
	org.PUT("", h.save)
	org.POST("/submit", h.submit)

	adm := r.Admin.Group("/organisers", authn.RequirePermission("organisers.review"))
	adm.GET("", h.adminList)
	adm.GET("/:userId", h.adminDetail)
	adm.POST("/:userId/approve", h.adminApprove)
	adm.POST("/:userId/reject", h.adminReject)
}

// EventHandlers implements app.EventSubscriber.
func (m *Module) EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{events.UserDeleted: m.svc.onUserDeleted}
}

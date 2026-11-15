package registration

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/events"
)

// Module is the registration module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
	h   *handler
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module {
	svc := newService(d)
	return &Module{d: d, svc: svc, h: &handler{svc: svc}}
}

// Name implements app.Module.
func (m *Module) Name() string { return "registration" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	r.Verified.POST("/events/:id/registrations", m.h.register)
	r.Verified.DELETE("/events/:id/registrations/me", m.h.withdraw)
	r.Verified.GET("/events/:id/registrations", m.h.forEvent)
	r.Verified.PATCH("/events/:id/registrations/:regId", m.h.patch)
	r.Verified.GET("/me/registrations", m.h.mine)

	r.Admin.GET("/events/:id/registrations", authn.RequirePermission("events.review"), m.h.adminForEvent)
}

// EventHandlers implements the app event-handler provider.
func (m *Module) EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.LocationIdentityChanged: m.svc.OnIdentityChanged,
	}
}

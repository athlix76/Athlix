package event

import (
	"context"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
)

// Module is the event module. See README.md in this folder.
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
func (m *Module) Name() string { return "event" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	r.Verified.POST("/events", m.h.create)
	r.Verified.GET("/events", m.h.list)
	r.Verified.GET("/events/featured", m.h.featured)
	r.Verified.GET("/events/eligibility/preview", m.h.preview)
	r.Verified.GET("/events/:id", m.h.get)
	r.Verified.PUT("/events/:id", m.h.update)
	r.Verified.GET("/events/:id/contact", m.h.contact)
	r.Verified.POST("/events/:id/cancel", m.h.cancel)
	r.Verified.POST("/events/:id/feature", m.h.feature)
	r.Verified.GET("/me/events", m.h.mine)

	perm := authn.RequirePermission("events.review")
	r.Admin.GET("/events", perm, m.h.adminList)
	r.Admin.GET("/events/:id", perm, m.h.adminGet)
	r.Admin.POST("/events/:id/approve", perm, m.h.adminApprove)
	r.Admin.POST("/events/:id/reject", perm, m.h.adminReason(m.svc.Reject))
	r.Admin.POST("/events/:id/remove", perm, m.h.adminReason(m.svc.Remove))
	r.Admin.POST("/events/:id/feature", perm, m.h.adminFeature(true))
	r.Admin.POST("/events/:id/unfeature", perm, m.h.adminFeature(false))
}

// Jobs implements the app job provider.
func (m *Module) Jobs() map[string]app.JobFunc {
	return map[string]app.JobFunc{
		"event.complete_past": func(ctx context.Context) error { return m.svc.CompletePast(ctx) },
	}
}

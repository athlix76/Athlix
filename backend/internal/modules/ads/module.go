package ads

import (
	"context"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
)

// Module is the ads module. See README.md in this folder.
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
func (m *Module) Name() string { return "ads" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	r.Verified.GET("/ads", m.h.serve)
	r.Verified.POST("/ads/:id/impression", m.h.track(false))
	r.Verified.POST("/ads/:id/click", m.h.track(true))

	perm := authn.RequirePermission("ads.write")
	r.Admin.GET("/ads", perm, m.h.adminList)
	r.Admin.POST("/ads", perm, m.h.adminCreate)
	r.Admin.PUT("/ads/:id", perm, m.h.adminUpdate)
	r.Admin.GET("/ads/:id/stats", perm, m.h.adminStats)
}

// Jobs implements app.JobProvider.
func (m *Module) Jobs() map[string]app.JobFunc {
	return map[string]app.JobFunc{
		"ads.end_campaigns": func(ctx context.Context) error { return m.svc.EndCampaigns(ctx) },
	}
}

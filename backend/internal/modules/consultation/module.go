package consultation

import "athlix/backend/internal/app"

// Module is the consultation module. See README.md in this folder.
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
func (m *Module) Name() string { return "consultation" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	r.Verified.GET("/coaches/:userId/consultation", m.h.getListing)
	r.Verified.GET("/me/consultation", m.h.myListing)
	r.Verified.PUT("/me/consultation", m.h.putListing)
	r.Verified.POST("/coaches/:userId/consultation-requests", m.h.createRequest)
	r.Verified.GET("/me/consultation-requests", m.h.listRequests)
	r.Verified.POST("/consultation-requests/:id/respond", m.h.respond)
	r.Verified.POST("/consultation-requests/:id/cancel", m.h.cancel)
	r.Verified.POST("/consultation-requests/:id/complete", m.h.complete)
}

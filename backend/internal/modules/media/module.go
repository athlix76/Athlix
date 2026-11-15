package media

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
)

// Module is the media module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module { return &Module{d: d, svc: newService(d)} }

// Name implements app.Module.
func (m *Module) Name() string { return "media" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := &handler{svc: m.svc}
	r.User.POST("/media/uploads", h.create)
	r.User.POST("/media/uploads/:id/confirm", h.confirm)
	r.Admin.POST("/media/uploads", authn.RequirePermission("ads.write"), h.adminCreate)
	r.Admin.POST("/media/uploads/:id/confirm", authn.RequirePermission("ads.write"), h.adminConfirm)
}

// Jobs implements app.JobProvider.
func (m *Module) Jobs() map[string]app.JobFunc {
	return map[string]app.JobFunc{"media.cleanup_pending": m.svc.CleanupPending}
}

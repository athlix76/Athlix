package auth

import (
	"athlix/backend/internal/app"
)

// Module is the auth module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module { return &Module{d: d, svc: newService(d)} }

// Name implements app.Module.
func (m *Module) Name() string { return "auth" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := &handler{svc: m.svc}
	r.Public.POST("/auth/otp/request", h.requestOTP)
	r.Public.POST("/auth/otp/verify", h.verifyOTP)
	r.Public.POST("/auth/refresh", h.refresh)
	r.User.POST("/auth/logout", h.logout)
}

// Jobs implements app.JobProvider.
func (m *Module) Jobs() map[string]app.JobFunc {
	return map[string]app.JobFunc{"auth.cleanup_tokens": m.svc.cleanupTokens}
}

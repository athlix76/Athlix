package verification

import (
	"net/http"
	"time"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
)

// Module is the verification module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
}

// providerTimeout bounds each DigiLocker / aggregator HTTP call.
const providerTimeout = 15 * time.Second

// New builds the module (no I/O).
func New(d *app.Deps) *Module {
	return &Module{d: d, svc: newService(d, &http.Client{Timeout: providerTimeout})}
}

// Name implements app.Module.
func (m *Module) Name() string { return "verification" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := &handler{svc: m.svc}
	r.User.POST("/verification/digilocker/start", h.start)
	r.Public.GET("/verification/digilocker/callback", h.callback)
	r.User.GET("/verification/status", h.status)
	r.User.POST("/verification/mock/complete", h.mock)
}

// EventHandlers implements the optional app interface.
func (m *Module) EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{events.UserDeleted: m.svc.onUserDeleted}
}

// Jobs implements the optional app interface.
func (m *Module) Jobs() map[string]app.JobFunc {
	return map[string]app.JobFunc{
		"verification.minor_recheck":      m.svc.MinorRecheck,
		"verification.purge_subject_hash": m.svc.PurgeSubjectHashes,
	}
}

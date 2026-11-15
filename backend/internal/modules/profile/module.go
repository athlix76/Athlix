package profile

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
)

// Module is the profile module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module { return &Module{d: d, svc: newService(d)} }

// Name implements app.Module.
func (m *Module) Name() string { return "profile" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := &handler{svc: m.svc}
	r.User.POST("/onboarding/profile", h.setup)

	r.Verified.GET("/profiles/:userId", h.view)
	r.Verified.GET("/profiles/:userId/stats", h.stats)
	r.Verified.GET("/profiles/:userId/achievements", h.achievements)
	r.Verified.PUT("/me/profile", h.update)
	r.Verified.GET("/me/profile-viewers", h.viewers)
	r.Verified.POST("/me/stats", h.createStat)
	r.Verified.PUT("/me/stats/:id", h.updateStat)
	r.Verified.DELETE("/me/stats/:id", h.deleteStat)
	r.Verified.POST("/me/achievements", h.createAchievement)
	r.Verified.PUT("/me/achievements/:id", h.updateAchievement)
	r.Verified.DELETE("/me/achievements/:id", h.deleteAchievement)
}

// EventHandlers implements app.EventSubscriber.
func (m *Module) EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{events.UserDeleted: m.svc.onUserDeleted}
}

package follow

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
)

// Module is the follow module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module { return &Module{d: d, svc: newService(d)} }

// Name implements app.Module.
func (m *Module) Name() string { return "follow" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := &handler{svc: m.svc}
	r.Verified.POST("/users/:userId/follow", h.follow)
	r.Verified.DELETE("/users/:userId/follow", h.unfollow)
	r.Verified.GET("/users/:userId/followers", h.followers)
	r.Verified.GET("/users/:userId/following", h.following)
	r.Verified.POST("/users/:userId/block", h.block)
	r.Verified.DELETE("/users/:userId/block", h.unblock)
	r.Verified.GET("/me/blocks", h.blocks)
}

// EventHandlers implements app.EventSubscriber.
func (m *Module) EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{events.UserDeleted: m.svc.onUserDeleted}
}

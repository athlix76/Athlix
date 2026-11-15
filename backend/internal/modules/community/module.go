// Package community owns user-created groups: conversations of type 'community' and their
// conversation_members rows (docs/api.md "Communities"). See README.md.
package community

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
)

// Module is the community module. It consumes no events and owns no jobs.
type Module struct {
	d   *app.Deps
	svc *Service
	h   *handler
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module {
	svc := NewService(d)
	return &Module{d: d, svc: svc, h: &handler{svc: svc}}
}

// Name implements app.Module.
func (m *Module) Name() string { return "community" }

// Service exposes the module's service.
func (m *Module) Service() *Service { return m.svc }

// Routes implements app.Module. App routes are on Verified; admin routes need communities.moderate.
func (m *Module) Routes(r app.Routes) {
	g := r.Verified
	g.POST("/communities", m.h.create)
	g.GET("/communities", m.h.browse)
	g.GET("/me/communities", m.h.mine)
	g.GET("/communities/:id", m.h.get)
	g.PUT("/communities/:id", m.h.update)
	g.DELETE("/communities/:id", m.h.remove)
	g.POST("/communities/:id/join", m.h.join)
	g.GET("/communities/:id/join-requests", m.h.joinRequests)
	g.POST("/communities/:id/join-requests/:userId/approve", m.h.approve)
	g.POST("/communities/:id/join-requests/:userId/reject", m.h.reject)
	g.POST("/communities/:id/invites", m.h.invite)
	g.POST("/communities/:id/invites/accept", m.h.accept)
	g.POST("/communities/:id/members/:userId/remove", m.h.removeMember)
	g.POST("/communities/:id/members/:userId/ban", m.h.banMember)
	g.PUT("/communities/:id/members/:userId/role", m.h.setRole)
	g.POST("/communities/:id/transfer-ownership", m.h.transferOwnership)

	mod := authn.RequirePermission("communities.moderate")
	r.Admin.GET("/communities", mod, m.h.adminList)
	r.Admin.POST("/communities/:id/remove", mod, m.h.adminRemove)
}

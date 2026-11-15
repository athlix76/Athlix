// Package feed owns posts, likes and comments (docs/api.md "Feed"). See README.md.
package feed

import "athlix/backend/internal/app"

// Module is the feed module. It consumes no events and owns no jobs.
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
func (m *Module) Name() string { return "feed" }

// Routes implements app.Module. Every feed route is on the Verified group.
func (m *Module) Routes(r app.Routes) {
	g := r.Verified
	g.GET("/feed", m.h.feed)
	g.GET("/users/:userId/posts", m.h.userPosts)
	g.POST("/posts", m.h.createPost)
	g.GET("/posts/:id", m.h.getPost)
	g.DELETE("/posts/:id", m.h.deletePost)
	g.POST("/posts/:id/like", m.h.like)
	g.DELETE("/posts/:id/like", m.h.unlike)
	g.GET("/posts/:id/comments", m.h.comments)
	g.POST("/posts/:id/comments", m.h.addComment)
	g.DELETE("/comments/:id", m.h.deleteComment)
}

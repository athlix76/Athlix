// Package search serves people search and the global search box. It is read-only: it reads
// users, profiles, events and conversations and writes nothing.
package search

import "athlix/backend/internal/app"

type Module struct {
	d   *app.Deps
	svc *Service
	h   *handler
}

func New(d *app.Deps) *Module {
	svc := newService(d)
	return &Module{d: d, svc: svc, h: &handler{svc: svc}}
}

func (m *Module) Name() string { return "search" }

func (m *Module) Routes(r app.Routes) {
	r.Verified.GET("/search/people", m.h.people)
	r.Verified.GET("/search", m.h.global)
}

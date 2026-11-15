// Package moderation owns reports and moderation actions, auto-hides heavily reported content
// and lets admins resolve reports. It may update ONLY the status column of users, events,
// posts, post_comments, messages and conversations (docs/architecture.md exception 1).
package moderation

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
)

type Module struct {
	d   *app.Deps
	svc *Service
	h   *handler
}

func New(d *app.Deps) *Module {
	svc := newService(d)
	return &Module{d: d, svc: svc, h: &handler{svc: svc}}
}

func (m *Module) Name() string { return "moderation" }

func (m *Module) Routes(r app.Routes) {
	r.Verified.POST("/reports", m.h.createReport)

	r.Admin.GET("/reports", authn.RequirePermission("reports.read"), m.h.listReports)
	r.Admin.POST("/reports/:id/resolve", authn.RequirePermission("reports.resolve"), m.h.resolve)
	r.Admin.GET("/moderation/actions", authn.RequirePermission("reports.read"), m.h.listActions)
	r.Admin.POST("/moderation/actions", authn.RequirePermission("reports.resolve"), m.h.createAction)
}

// Package locationidentity owns native / residence location identities, the proof review
// queue and the states / districts reference tables. The eligibility rule itself lives in
// platform/eligibility (shared by event, registration and search).
package locationidentity

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/events"
)

const (
	permReview     = "location.review"
	permConfigRead = "config.read"
	permGeoWrite   = "geo.write"
)

type Module struct {
	d   *app.Deps
	svc *Service
	h   *handler
}

func New(d *app.Deps) *Module {
	// This module owns states / districts, so it attaches the shared version counter that
	// makes its edits visible on every instance (platform/eligibility Places.Bump).
	if d.Eligibility != nil && d.Redis != nil {
		d.Eligibility.Places().UseVersionStore(d.Redis, 0)
	}
	svc := newService(d)
	return &Module{d: d, svc: svc, h: &handler{svc: svc}}
}

func (m *Module) Name() string { return "locationidentity" }

func (m *Module) Routes(r app.Routes) {
	r.User.GET("/geo/states", m.h.states)
	r.User.GET("/geo/districts", m.h.districts)

	r.Verified.GET("/me/location-identities", m.h.mine)
	r.Verified.POST("/me/location-identities", m.h.submit)
	r.Verified.DELETE("/me/location-identities/:id", m.h.withdraw)

	review := authn.RequirePermission(permReview)
	r.Admin.GET("/location-identities", review, m.h.queue)
	r.Admin.GET("/location-identities/:id", review, m.h.detail)
	r.Admin.POST("/location-identities/:id/approve", review, m.h.approve)
	r.Admin.POST("/location-identities/:id/reject", review, m.h.reject)
	r.Admin.POST("/location-identities/:id/revoke", review, m.h.revoke)

	r.Admin.GET("/geo/states", authn.RequirePermission(permConfigRead), m.h.adminStates)
	r.Admin.GET("/geo/districts", authn.RequirePermission(permConfigRead), m.h.adminDistricts)
	r.Admin.POST("/geo/districts", authn.RequirePermission(permGeoWrite), m.h.createDistrict)
	r.Admin.PUT("/geo/districts/:id", authn.RequirePermission(permGeoWrite), m.h.updateDistrict)
}

// EventHandlers implements app.EventSubscriber.
func (m *Module) EventHandlers() map[string]events.Handler { return m.svc.handlers() }

// Jobs implements app.JobProvider.
func (m *Module) Jobs() map[string]app.JobFunc { return m.svc.jobs() }

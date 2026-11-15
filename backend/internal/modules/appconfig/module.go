package appconfig

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
)

// Module is the appconfig module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module { return &Module{d: d, svc: newService(d)} }

// Name implements app.Module.
func (m *Module) Name() string { return "appconfig" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := &handler{svc: m.svc}
	r.Public.GET("/bootstrap", h.bootstrap)
	r.Public.GET("/policies/:docType", h.policy)

	read := authn.RequirePermission("config.read")
	write := authn.RequirePermission("config.write")
	policies := authn.RequirePermission("policies.write")
	sports := authn.RequirePermission("sports.write")

	r.Admin.GET("/config", read, h.configList)
	r.Admin.PUT("/config/:key", write, h.putConfig)
	r.Admin.GET("/config/:key/history", read, h.configHistory)

	r.Admin.GET("/policies", read, h.policies)
	r.Admin.POST("/policies", policies, h.createPolicy)
	r.Admin.POST("/policies/:id/activate", policies, h.activatePolicy)

	r.Admin.GET("/verification-requirements", read, h.requirements)
	r.Admin.POST("/verification-requirements", write, h.createRequirement)
	r.Admin.PUT("/verification-requirements/:id", write, h.updateRequirement)

	r.Admin.GET("/organiser-steps", read, h.steps)
	r.Admin.POST("/organiser-steps", write, h.createStep)
	r.Admin.PUT("/organiser-steps/:id", write, h.updateStep)

	r.Admin.GET("/sports", read, h.sports)
	r.Admin.POST("/sports", sports, h.createSport)
	r.Admin.PUT("/sports/:id", sports, h.updateSport)
	r.Admin.POST("/sports/:id/positions", sports, h.createPosition)
	r.Admin.PUT("/positions/:id", sports, h.updatePosition)
	r.Admin.POST("/sports/:id/stat-templates", sports, h.createTemplate)
	r.Admin.PUT("/stat-templates/:id", sports, h.updateTemplate)
}

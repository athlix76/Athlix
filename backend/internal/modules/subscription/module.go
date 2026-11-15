package subscription

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
)

// Module is the subscription module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
	h   *handler
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module {
	svc := newService(d)
	return &Module{d: d, svc: svc, h: &handler{svc: svc}}
}

// Name implements app.Module.
func (m *Module) Name() string { return "subscription" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	h := m.h
	r.Verified.GET("/premium/plans", h.plans)
	r.Verified.GET("/me/subscription", h.mine)
	r.Verified.POST("/premium/purchases/verify", h.verify)
	r.Verified.POST("/premium/purchases/restore", h.restore)
	r.Verified.POST("/premium/mock/activate", h.mockActivate)

	r.Webhooks.POST("/google-play", h.googlePlay)
	r.Webhooks.POST("/app-store", h.appStore)

	r.Admin.GET("/plans", authn.RequirePermission("config.read"), h.adminPlans)
	r.Admin.POST("/plans", authn.RequirePermission("plans.write"), h.createPlan)
	r.Admin.PUT("/plans/:id", authn.RequirePermission("plans.write"), h.updatePlan)
	r.Admin.PUT("/plans/:id/benefits", authn.RequirePermission("plans.write"), h.replaceBenefits)
	r.Admin.GET("/subscriptions", authn.RequirePermission("payments.read"), h.adminSubscriptions)
	r.Admin.POST("/subscriptions/grant", authn.RequirePermission("plans.write"), h.grant)
}

// Jobs implements app.JobProvider.
func (m *Module) Jobs() map[string]app.JobFunc {
	return map[string]app.JobFunc{"subscription.expire": m.svc.ExpireDue}
}

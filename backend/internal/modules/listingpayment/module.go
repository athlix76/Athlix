package listingpayment

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/events"
)

// Module is the listingpayment module. See README.md in this folder.
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
func (m *Module) Name() string { return "listingpayment" }

// Routes implements app.Module.
func (m *Module) Routes(r app.Routes) {
	r.Verified.GET("/listing/quote", m.h.quote)
	r.Verified.POST("/events/:id/listing/quote", m.h.eventQuote)
	r.Verified.POST("/events/:id/listing/order", m.h.order)
	r.Verified.GET("/events/:id/listing/order", m.h.latestOrder)
	r.Verified.POST("/events/:id/listing/confirm", m.h.confirm)
	r.Verified.POST("/listing/mock/pay", m.h.mockPay)

	r.Webhooks.POST("/razorpay", m.h.webhook)

	r.Admin.GET("/listing-orders", authn.RequirePermission("payments.read"), m.h.adminOrders)
	r.Admin.POST("/listing-orders/:id/refund", authn.RequirePermission("payments.refund"), m.h.adminRefund)
	r.Admin.GET("/fee-rules", authn.RequirePermission("config.read"), m.h.feeRules)
	r.Admin.POST("/fee-rules", authn.RequirePermission("fees.write"), m.h.createRule)
	r.Admin.PUT("/fee-rules/:id", authn.RequirePermission("fees.write"), m.h.updateRule)
	r.Admin.POST("/fee-rules/preview", authn.RequirePermission("config.read"), m.h.preview)
}

// EventHandlers implements the app event-handler provider.
func (m *Module) EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.EventCancelled: m.svc.OnCancelled,
		events.EventRejected:  m.svc.OnRejected,
		events.EventUpdated:   m.svc.OnUpdated,
	}
}

// Jobs implements the app job provider.
func (m *Module) Jobs() map[string]app.JobFunc {
	return map[string]app.JobFunc{"listing.expire_orders": m.svc.ExpireOrders}
}

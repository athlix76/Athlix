// Package notification owns in-app notifications and device tokens. It turns domain events
// into notification rows and FCM pushes, runs event reminders and admin broadcasts.
package notification

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/push"
)

// Module is the notification feature module.
type Module struct {
	d   *app.Deps
	svc *Service
}

// New builds the module. The FCM sender is created lazily from the `fcm` secret.
func New(d *app.Deps) *Module { return &Module{d: d, svc: newService(d)} }

// Name implements app.Module.
func (m *Module) Name() string { return "notification" }

// Service exposes the module's service (used by tests and the worker).
func (m *Module) Service() *Service { return m.svc }

// SetPushSender replaces the push sender (tests inject *push.Fake).
func (m *Module) SetPushSender(s push.Sender) { m.svc.setSender(s) }

// Routes registers the HTTP routes (docs/api.md "Devices", "Notifications", admin broadcast).
func (m *Module) Routes(r app.Routes) {
	h := &handler{s: m.svc}
	r.User.POST("/notifications/devices", h.registerDevice)
	r.User.DELETE("/notifications/devices", h.deleteDevice)
	r.Verified.GET("/notifications", h.list)
	r.Verified.GET("/notifications/unread-count", h.unreadCount)
	r.Verified.POST("/notifications/read", h.read)
	r.Admin.POST("/notifications/broadcast", authn.RequirePermission("notifications.broadcast"), h.broadcast)
}

// EventHandlers implements the optional event consumer interface.
func (m *Module) EventHandlers() map[string]events.Handler { return m.svc.handlers() }

// Jobs implements the optional scheduled-job interface.
func (m *Module) Jobs() map[string]app.JobFunc {
	return map[string]app.JobFunc{"notification.event_reminders": m.svc.EventReminders}
}

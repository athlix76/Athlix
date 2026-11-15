// Package chat owns conversations (direct, community, event), members and messages: REST,
// the WebSocket protocol (local route + cmd/ws), fan-out and event-group membership.
package chat

import (
	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/events"
)

// Module is the chat module. See README.md in this folder.
type Module struct {
	d   *app.Deps
	svc *Service
}

// New builds the module (no I/O).
func New(d *app.Deps) *Module { return &Module{d: d, svc: NewService(d)} }

// Name implements app.Module.
func (m *Module) Name() string { return "chat" }

// Service exposes the shared chat service (send path, tickets, frames).
func (m *Module) Service() *Service { return m.svc }

// Routes implements app.Module (docs/api.md "Chat" and the admin messages route).
func (m *Module) Routes(r app.Routes) {
	h := &handler{s: m.svc}
	v := r.Verified
	v.POST("/chat/ws-ticket", h.wsTicket)
	v.GET("/chat/conversations", h.conversations)
	v.GET("/chat/unread-count", h.unreadCount)
	v.POST("/chat/direct", h.direct)
	v.GET("/chat/conversations/:id", h.conversation)
	v.GET("/chat/conversations/:id/messages", h.messages)
	v.POST("/chat/conversations/:id/messages", h.send)
	v.POST("/chat/conversations/:id/read", h.read)
	v.POST("/chat/conversations/:id/mute", h.mute)
	v.POST("/chat/conversations/:id/leave", h.leave)
	v.GET("/chat/conversations/:id/members", h.members)
	v.DELETE("/chat/messages/:id", h.deleteMessage)
	v.GET("/events/:id/group", h.eventGroup)

	r.Admin.GET("/conversations/:id/messages", authn.RequirePermission("moderation.read_messages"), h.adminMessages)

	if m.d.Cfg.IsLocal() && r.Engine != nil {
		// Local dev only: the single-use ticket is the authentication (AWS uses API Gateway + cmd/ws).
		r.Engine.GET("/v1/chat/ws", m.svc.serveLocalWS)
	}
}

// EventHandlers implements app.EventSubscriber.
func (m *Module) EventHandlers() map[string]events.Handler { return m.svc.handlers() }

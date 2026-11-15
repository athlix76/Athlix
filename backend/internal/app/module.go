// Package app wires the platform into one Deps value and defines the contract every module
// implements. Modules live in internal/modules/<name> and are listed in internal/modules/registry.go.
package app

import (
	"context"

	"athlix/backend/internal/platform/events"
)

// Module is implemented by every package under internal/modules.
//
//	type Module struct{ d *app.Deps; svc *Service }
//	func New(d *app.Deps) *Module          // build repo + service; no I/O
//	func (m *Module) Name() string         // "event"
//	func (m *Module) Routes(r app.Routes)  // register handlers on the right groups
type Module interface {
	Name() string
	Routes(r Routes)
}

// EventSubscriber is optionally implemented by modules that react to domain events.
// Handlers must be idempotent (SQS delivers at least once).
type EventSubscriber interface {
	EventHandlers() map[string]events.Handler
}

// JobFunc is a scheduled job (EventBridge -> cmd/worker, or `go run ./cmd/worker -job name` locally).
type JobFunc func(ctx context.Context) error

// JobProvider is optionally implemented by modules that own scheduled jobs.
// Job names are a closed list in docs/architecture.md#scheduled-jobs.
type JobProvider interface {
	Jobs() map[string]JobFunc
}

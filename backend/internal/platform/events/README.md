# `internal/platform/events`

Package events is the async domain-event bus.

Modules publish facts ("event.published", "message.created", ...) after their DB
write commits; other modules react in EventHandlers (notifications, counters,
group creation). Locally (EVENTS_QUEUE_URL empty) events are dispatched
in-process on a goroutine. In AWS they go to SQS and cmd/worker dispatches them.
Handlers MUST be idempotent (SQS is at-least-once). Payloads carry ids, never
PII or secrets.

The full catalog is in docs/architecture.md#domain-events.


## Exported API

```go
type CommunityJoinPayload struct
type ConsultationPayload struct
type Dispatcher struct
func NewDispatcher() *Dispatcher
func (d *Dispatcher) Dispatch(ctx context.Context, e Event) error
func (d *Dispatcher) On(typ string, h Handler)
type Event struct
func New(typ string, actorID int64, payload any) Event
func (e Event) Decode(out any) error
type EventPayload struct
type FollowPayload struct
type Handler func(ctx context.Context, e Event) error
type ListingPayload struct
type Local struct
func (l *Local) Publish(ctx context.Context, e Event) error
func (l *Local) Wait()
type MessagePayload struct
type Nop struct{}
func (Nop) Publish(context.Context, Event) error
type OrganiserPayload struct
type PostPayload struct
type Publisher interface
type RegistrationPayload struct
type ReportPayload struct
type SQS struct
func NewSQS(cfg aws.Config, queueURL string) *SQS
func (s *SQS) Publish(ctx context.Context, e Event) error
type SubscriptionPayload struct
type UserPayload struct
type VerificationPayload struct
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/events`). Keep the package comment true and update this file when the API changes._

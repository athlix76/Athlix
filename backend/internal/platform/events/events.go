// Package events is the async domain-event bus.
//
// Modules publish facts ("event.published", "message.created", ...) after their DB write
// commits; other modules react in EventHandlers (notifications, counters, group creation).
// Locally (EVENTS_QUEUE_URL empty) events are dispatched in-process on a goroutine. In AWS
// they go to SQS and cmd/worker dispatches them. Handlers MUST be idempotent (SQS is
// at-least-once). Payloads carry ids, never PII or secrets.
//
// The full catalog is in docs/architecture.md#domain-events.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"athlix/backend/internal/platform/logger"
)

// Event types (closed list; add new ones here and in docs/architecture.md).
const (
	UserRegistered          = "user.registered"
	UserDeleted             = "user.deleted"
	VerificationCompleted   = "verification.completed"
	GuardianConsentGranted  = "guardian.consent_granted"
	GuardianConsentRevoked  = "guardian.consent_revoked"
	FollowCreated           = "follow.created"
	EventSubmitted          = "event.submitted"
	EventPublished          = "event.published"
	EventRejected           = "event.rejected"
	EventCancelled          = "event.cancelled"
	EventUpdated            = "event.updated"
	EventCompleted          = "event.completed"
	RegistrationCreated     = "registration.created"
	RegistrationWithdrawn   = "registration.withdrawn"
	RegistrationPromoted    = "registration.promoted"
	RegistrationPaymentMark = "registration.payment_marked"
	ListingPaid             = "listing.paid"
	ListingRefunded         = "listing.refunded"
	OrganiserApproved       = "organiser.approved"
	OrganiserRejected       = "organiser.rejected"
	MessageCreated          = "message.created"
	PostLiked               = "post.liked"
	PostCommented           = "post.commented"
	ConsultationRequested   = "consultation.requested"
	ConsultationResponded   = "consultation.responded"
	ReportCreated           = "report.created"
	SubscriptionActivated   = "subscription.activated"
	SubscriptionExpired     = "subscription.expired"
	CommunityJoinRequested  = "community.join_requested"
	CommunityJoinApproved   = "community.join_approved"

	// Location identity (docs/architecture.md#location-eligibility).
	LocationIdentityChanged        = "location_identity.changed"        // verified / rejected / expired / revoked / withdrawn / superseded
	LocationIdentityExpiring       = "location_identity.expiring"       // residence expires within location.expiry_reminder_days
	RegistrationEligibilityFlagged = "registration.eligibility_flagged" // a registration's qualifying identity stopped being valid
)

// Event is one domain fact. Payload is a small JSON object of ids/flags.
type Event struct {
	Type       string          `json:"type"`
	ActorID    int64           `json:"actorId,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt time.Time       `json:"occurredAt"`
}

// New builds an Event from any JSON-marshalable payload.
func New(typ string, actorID int64, payload any) Event {
	b, _ := json.Marshal(payload)
	return Event{Type: typ, ActorID: actorID, Payload: b, OccurredAt: time.Now().UTC()}
}

// Decode unmarshals the payload into out.
func (e Event) Decode(out any) error { return json.Unmarshal(e.Payload, out) }

// Handler reacts to one event. Return an error to retry (SQS) / log (local).
type Handler func(ctx context.Context, e Event) error

// Publisher is what modules depend on.
type Publisher interface {
	Publish(ctx context.Context, e Event) error
}

// Dispatcher routes events to registered handlers (used by Local and cmd/worker).
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// NewDispatcher builds an empty dispatcher.
func NewDispatcher() *Dispatcher { return &Dispatcher{handlers: map[string][]Handler{}} }

// On registers h for type typ.
func (d *Dispatcher) On(typ string, h Handler) {
	d.mu.Lock()
	d.handlers[typ] = append(d.handlers[typ], h)
	d.mu.Unlock()
}

// Dispatch runs every handler for e; returns the joined errors.
func (d *Dispatcher) Dispatch(ctx context.Context, e Event) error {
	d.mu.RLock()
	hs := d.handlers[e.Type]
	d.mu.RUnlock()
	var errs []error
	for _, h := range hs {
		if err := h(ctx, e); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Local dispatches in-process on a goroutine (ENV=local, tests).
type Local struct {
	D    *Dispatcher
	Sync bool // tests: dispatch synchronously
	wg   sync.WaitGroup
}

func (l *Local) Publish(ctx context.Context, e Event) error {
	run := func(ctx context.Context) {
		if err := l.D.Dispatch(ctx, e); err != nil {
			logger.From(ctx).Error("event handler failed", slog.String("type", e.Type), slog.String("err", err.Error()))
		}
	}
	if l.Sync {
		run(ctx)
		return nil
	}
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		run(context.WithoutCancel(ctx))
	}()
	return nil
}

// Wait blocks until in-flight async dispatches finish (tests, shutdown).
func (l *Local) Wait() { l.wg.Wait() }

// SQS publishes to the events queue (AWS).
type SQS struct {
	client *sqs.Client
	url    string
}

// NewSQS builds the SQS publisher.
func NewSQS(cfg aws.Config, queueURL string) *SQS {
	return &SQS{client: sqs.NewFromConfig(cfg), url: queueURL}
}

func (s *SQS) Publish(ctx context.Context, e Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = s.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(s.url),
		MessageBody: aws.String(string(b)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"type": {DataType: aws.String("String"), StringValue: aws.String(e.Type)},
		},
	})
	if err != nil {
		return errors.New("events: sqs publish failed")
	}
	return nil
}

// Nop drops events (rarely useful; tests that ignore events).
type Nop struct{}

func (Nop) Publish(context.Context, Event) error { return nil }

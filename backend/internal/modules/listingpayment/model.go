package listingpayment

import (
	"time"

	"athlix/backend/internal/modules/event"
	"athlix/backend/internal/platform/userdir"
)

// Seed values of the settings this module reads (docs/config-system.md). Go default = seed.
const (
	defProvider        = providerMock
	defOrderTTLMinutes = 30
	defRefundBefore    = 100
	defRefundAfter     = 0
	defRefundRejection = 100
	defRequireApproval = true
	defMinHoursBefore  = 24
	defAllowFree       = true
	defMaxSlotsLimit   = 1024
	defPageSize        = 20
)

// Contract limits.
const (
	maxTeamMembers = 30 // same bound the event module applies to team sizes
	maxBp          = 10000
	maxReasonLen   = 255 // listing_orders.refund_reason
	maxRuleNameLen = 80  // listing_fee_rules.name
	maxUint32      = 1<<32 - 1
	maxWebhookBody = 1 << 20
	batchSize      = 500 // CLAUDE.md: jobs and handlers touch ≤ 500 rows per batch
)

// Payment providers (listing_orders.provider). "none" is a zero-total order paid at once.
const (
	providerRazorpay = "razorpay"
	providerMock     = "mock"
	providerNone     = "none"
)

// Order statuses (listing_orders.status).
const (
	stCreated   = "created"
	stPaid      = "paid"
	stFailed    = "failed"
	stExpired   = "expired"
	stRefunded  = "refunded"
	stPartially = "partially_refunded"
)

var (
	orderStatuses = map[string]bool{stCreated: true, stPaid: true, stFailed: true, stExpired: true, stRefunded: true, stPartially: true}
	eventTypes    = map[string]bool{"tournament": true, "trial": true, "camp": true, "match": true, "other": true}
)

// RefundPolicy is the three listing.refund.* percentages shown before payment.
type RefundPolicy struct {
	BeforeFirstRegistrationPercent int `json:"beforeFirstRegistrationPercent"`
	AfterRegistrationsPercent      int `json:"afterRegistrationsPercent"`
	OnAdminRejectionPercent        int `json:"onAdminRejectionPercent"`
}

// Quote is ListingQuote.
type Quote struct {
	FeeRuleID              int64        `json:"feeRuleId"`
	RuleName               string       `json:"ruleName"`
	EntryFeePaise          int64        `json:"entryFeePaise"`
	MaxSlots               int64        `json:"maxSlots"`
	GrossEntryPaise        int64        `json:"grossEntryPaise"`
	PercentBp              int64        `json:"percentBp"`
	MinFeePaise            int64        `json:"minFeePaise"`
	MaxFeePaise            *int64       `json:"maxFeePaise"`
	IsFreeEvent            bool         `json:"isFreeEvent"`
	BaseFeePaise           int64        `json:"baseFeePaise"`
	GstBp                  int64        `json:"gstBp"`
	GstPaise               int64        `json:"gstPaise"`
	TotalPaise             int64        `json:"totalPaise"`
	RefundPolicy           RefundPolicy `json:"refundPolicy"`
	RefundPolicyDocumentID *int64       `json:"refundPolicyDocumentId"`
}

// Order is ListingOrder.
type Order struct {
	OrderID         int64      `json:"orderId"`
	EventID         int64      `json:"eventId"`
	Status          string     `json:"status"`
	Quote           Quote      `json:"quote"`
	Provider        string     `json:"provider"`
	ProviderOrderID *string    `json:"providerOrderId"`
	ProviderKeyID   *string    `json:"providerKeyId"`
	RefundPaise     int64      `json:"refundPaise"`
	ExpiresAt       time.Time  `json:"expiresAt"`
	PaidAt          *time.Time `json:"paidAt"`
	EventStatus     string     `json:"eventStatus"`
}

// AdminOrder is ListingOrder + {organiser}.
type AdminOrder struct {
	Order
	Organiser *userdir.Summary `json:"organiser"`
}

// FeeRule is one listing_fee_rules row.
type FeeRule struct {
	ID                int64      `json:"id"`
	Name              string     `json:"name"`
	SportID           *int64     `json:"sportId"`
	EventType         *string    `json:"eventType"`
	PercentBp         int64      `json:"percentBp"`
	MinFeePaise       int64      `json:"minFeePaise"`
	MaxFeePaise       *int64     `json:"maxFeePaise"`
	FreeEventFeePaise int64      `json:"freeEventFeePaise"`
	GstBp             int64      `json:"gstBp"`
	Priority          int64      `json:"priority"`
	Active            bool       `json:"active"`
	EffectiveFrom     time.Time  `json:"effectiveFrom"`
	EffectiveTo       *time.Time `json:"effectiveTo"`
}

// isDefault reports whether the rule matches every sport and type.
func (r FeeRule) isDefault() bool { return r.SportID == nil && r.EventType == nil }

// FeeRuleInput is POST (all required fields) and PUT (partial; null clears a nullable field).
type FeeRuleInput struct {
	Name              event.Opt[string]    `json:"name"`
	SportID           event.Opt[int64]     `json:"sportId"`
	EventType         event.Opt[string]    `json:"eventType"`
	PercentBp         event.Opt[int64]     `json:"percentBp"`
	MinFeePaise       event.Opt[int64]     `json:"minFeePaise"`
	MaxFeePaise       event.Opt[int64]     `json:"maxFeePaise"`
	FreeEventFeePaise event.Opt[int64]     `json:"freeEventFeePaise"`
	GstBp             event.Opt[int64]     `json:"gstBp"`
	Priority          event.Opt[int64]     `json:"priority"`
	Active            event.Opt[bool]      `json:"active"`
	EffectiveFrom     event.Opt[time.Time] `json:"effectiveFrom"`
	EffectiveTo       event.Opt[time.Time] `json:"effectiveTo"`
}

// QuoteInput is GET /listing/quote and POST /admin/fee-rules/preview.
type QuoteInput struct {
	SportID       int64  `json:"sportId"`
	EventType     string `json:"eventType"`
	EntryFeePaise int64  `json:"entryFeePaise"`
	MaxSlots      int64  `json:"maxSlots"`
}

// ConfirmInput is POST /events/:id/listing/confirm.
type ConfirmInput struct {
	OrderID           int64  `json:"orderId"`
	ProviderPaymentID string `json:"providerPaymentId"`
	ProviderSignature string `json:"providerSignature"`
}

// MockPayInput is POST /listing/mock/pay.
type MockPayInput struct {
	OrderID int64 `json:"orderId"`
}

// RefundInput is POST /admin/listing-orders/:id/refund.
type RefundInput struct {
	AmountPaise int64  `json:"amountPaise"`
	Reason      string `json:"reason"`
}

// OrderFilter is GET /admin/listing-orders.
type OrderFilter struct {
	Status  string
	EventID int64
	From    *time.Time
	To      *time.Time
}

// orderRec is one listing_orders row (plus the rule name/min/max and the event status).
type orderRec struct {
	ID                int64
	EventID           int64
	OrganiserID       int64
	FeeRuleID         int64
	EntryFeePaise     int64
	MaxSlots          int64
	PercentBp         int64
	GstBp             int64
	BaseFeePaise      int64
	GstPaise          int64
	TotalPaise        int64
	Provider          string
	ProviderOrderID   *string
	ProviderPaymentID *string
	Status            string
	RefundPaise       int64
	ProviderRefundID  *string
	PaidAt            *time.Time
	ExpiresAt         time.Time
	CreatedAt         time.Time
	RuleName          string
	MinFeePaise       int64
	MaxFeePaise       *int64
	EventStatus       string
}

// open reports whether the order can still be paid.
func (o orderRec) open() bool { return o.Status == stCreated || o.Status == stFailed }

// settled reports whether a payment was already recorded on the order.
func (o orderRec) settled() bool {
	return o.Status == stPaid || o.Status == stRefunded || o.Status == stPartially
}

// sameQuote reports whether the order's snapshot equals q.
func (o orderRec) sameQuote(q Quote) bool {
	return o.FeeRuleID == q.FeeRuleID && o.EntryFeePaise == q.EntryFeePaise && o.MaxSlots == q.MaxSlots &&
		o.PercentBp == q.PercentBp && o.GstBp == q.GstBp && o.BaseFeePaise == q.BaseFeePaise &&
		o.GstPaise == q.GstPaise && o.TotalPaise == q.TotalPaise
}

// evRow is the part of an events row this module reads (events is owned by event; this
// module writes only events.status / published_at on payment and expiry).
type evRow struct {
	ID                 int64
	OrganiserID        int64
	SportID            int64
	EventType          string
	EntryFeePaise      int64
	MaxSlots           int64
	Status             string
	StartsAt           time.Time
	EndsAt             time.Time
	Deadline           time.Time
	ParticipantType    string
	TeamSizeMin        *int
	TeamSizeMax        *int
	RegistrationsCount int
}

// outbox collects domain events of one transaction; they are published after commit.
type outbox struct {
	paid     []orderRec
	refunded []refunded
	statuses []statusChange
}

type refunded struct {
	o      orderRec
	amount int64
}

type statusChange struct {
	typ string // events.EventSubmitted | events.EventPublished
	ev  evRow
}

package subscription

import (
	"encoding/json"
	"time"

	"athlix/backend/internal/platform/entitlements"
	"athlix/backend/internal/platform/userdir"
)

// Stores (subscriptions.store) and webhook providers (payment_webhook_events.provider).
const (
	StoreGooglePlay = "google_play"
	StoreAppStore   = "app_store"
	StoreManual     = "manual"
)

// Subscription statuses (subscriptions.status).
const (
	StatusActive    = "active"
	StatusGrace     = "grace"
	StatusExpired   = "expired"
	StatusCancelled = "cancelled"
	StatusRefunded  = "refunded"
)

// Webhook event statuses (payment_webhook_events.status).
const (
	webhookReceived  = "received"
	webhookProcessed = "processed"
	webhookIgnored   = "ignored"
	webhookFailed    = "failed"
)

// jobBatch is the max rows one job run touches (docs/architecture.md#scheduled-jobs).
const jobBatch = 500

// defaultPageSize is the admin list page size when ?limit is absent.
const defaultPageSize = 20

var (
	audiences      = map[string]bool{"player": true, "coach": true, "organiser": true}
	billingPeriods = map[string]int{"monthly": 1, "quarterly": 3, "yearly": 12} // months per period
	statuses       = map[string]bool{StatusActive: true, StatusGrace: true, StatusExpired: true, StatusCancelled: true, StatusRefunded: true}
)

// benefitKinds is the closed benefit list (docs/config-system.md#premium-benefits-closed-list):
// "bool" keys take `true`, "int" keys take a positive integer.
var benefitKinds = map[string]string{
	entitlements.ConsultationListing:   "bool",
	entitlements.ProfileBoost:          "bool",
	entitlements.AdvancedStats:         "bool",
	entitlements.SeeProfileViewers:     "bool",
	entitlements.AdvancedSearchFilters: "bool",
	entitlements.ExtraCommunities:      "int",
	entitlements.FeaturedEvents:        "int",
	entitlements.VerifiedCoachBadge:    "bool",
}

// Benefit is one plan benefit as shown to clients.
type Benefit struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
	Label string          `json:"label"`
}

// Plan is docs/api.md "Plan".
type Plan struct {
	ID                    int64     `json:"id"`
	Code                  string    `json:"code"`
	Audience              string    `json:"audience"`
	Name                  string    `json:"name"`
	Description           *string   `json:"description"`
	PricePaise            int64     `json:"pricePaise"`
	BillingPeriod         string    `json:"billingPeriod"`
	StoreProductIDAndroid *string   `json:"storeProductIdAndroid"`
	StoreProductIDIos     *string   `json:"storeProductIdIos"`
	Benefits              []Benefit `json:"benefits"`
}

// AdminPlan is Plan + {active, sortOrder}.
type AdminPlan struct {
	Plan
	Active    bool `json:"active"`
	SortOrder int  `json:"sortOrder"`
}

// Subscription is docs/api.md "Subscription".
type Subscription struct {
	ID        int64     `json:"id"`
	Plan      Plan      `json:"plan"`
	Store     string    `json:"store"`
	Status    string    `json:"status"`
	AutoRenew bool      `json:"autoRenew"`
	StartedAt time.Time `json:"startedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// AdminSubscription is Subscription + {user}.
type AdminSubscription struct {
	Subscription
	User *userdir.Summary `json:"user"`
}

// MySubscription is the GET /me/subscription response.
type MySubscription struct {
	Active   *Subscription              `json:"active"`
	Benefits map[string]json.RawMessage `json:"benefits"`
}

// benefitRow is a premium_plan_benefits row.
type benefitRow struct {
	PlanID    int64           `json:"-"`
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Label     string          `json:"label"`
	SortOrder int             `json:"sortOrder"`
}

// subRow is a subscriptions row.
type subRow struct {
	ID                 int64
	UserID             int64
	PlanID             int64
	Store              string
	StoreTransactionID *string
	Status             string
	AutoRenew          bool
	StartedAt          time.Time
	ExpiresAt          time.Time
}

func isLive(status string) bool { return status == StatusActive || status == StatusGrace }

// ---- requests ----

type verifyRequest struct {
	Store         string `json:"store"`
	ProductID     string `json:"productId"`
	PurchaseToken string `json:"purchaseToken"`
	TransactionID string `json:"transactionId"`
}

type mockActivateRequest struct {
	PlanID int64 `json:"planId"`
}

// planInput is the admin plan body. On PUT a nil field means "unchanged"; an empty string
// for description / store product ids clears the value.
type planInput struct {
	Code                  *string `json:"code"`
	Audience              *string `json:"audience"`
	Name                  *string `json:"name"`
	Description           *string `json:"description"`
	PricePaise            *int64  `json:"pricePaise"`
	BillingPeriod         *string `json:"billingPeriod"`
	StoreProductIDAndroid *string `json:"storeProductIdAndroid"`
	StoreProductIDIos     *string `json:"storeProductIdIos"`
	Active                *bool   `json:"active"`
	SortOrder             *int    `json:"sortOrder"`
}

type benefitInput struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Label     string          `json:"label"`
	SortOrder int             `json:"sortOrder"`
}

type grantRequest struct {
	UserID int64  `json:"userId"`
	PlanID int64  `json:"planId"`
	Days   int    `json:"days"`
	Reason string `json:"reason"`
}

type adminSubFilter struct {
	UserID int64
	PlanID int64
	Status string
}

// restoreRequest is POST /premium/purchases/restore: the purchases the store SDK returned on
// this device ("Restore purchases"). Each one is re-verified with the store, as in verify.
type restoreRequest struct {
	Purchases []verifyRequest `json:"purchases"`
}

// RestoreResult is the restore response: how many purchases were linked to the caller and
// the caller's subscription afterwards.
type RestoreResult struct {
	Restored     int            `json:"restored"`
	NotRestored  int            `json:"notRestored"`
	Subscription MySubscription `json:"subscription"`
}

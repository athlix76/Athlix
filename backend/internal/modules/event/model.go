package event

import (
	"encoding/json"
	"time"

	"athlix/backend/internal/platform/userdir"
)

// Seed values of the event.* settings (docs/config-system.md). The Go default equals the seed.
const (
	defRequireApproval  = true
	defShowContact      = "after_registration"
	defMaxActive        = 20
	defMinHoursBefore   = 24
	defAllowFree        = true
	defMaxSlotsLimit    = 1024
	defContactReveals   = 50
	defFeaturedListSize = 10
	defMaxRadiusKm      = 200
	defEligibilityGov   = "native"
	defEligibilityPriv  = "open"
	defPageSize         = 20
)

// maxTeamMembers is the contract limit on team members (docs/api.md, Registrations).
const maxTeamMembers = 30

// Event statuses.
const (
	stDraft          = "draft"
	stPendingPayment = "pending_payment"
	stPendingReview  = "pending_review"
	stPublished      = "published"
	stRejected       = "rejected"
	stCancelled      = "cancelled"
	stCompleted      = "completed"
	stRemoved        = "removed"
)

// ist is India Standard Time; calendar boundaries (dates in filters, featured month) use it.
var ist = time.FixedZone("IST", 5*3600+1800)

// Opt is a JSON field that remembers whether it was present (Set) and non-null (Valid).
// PUT /events/:id is partial: an absent field is unchanged, null clears it.
type Opt[T any] struct {
	Set   bool
	Valid bool
	V     T
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Valid = false
		return nil
	}
	o.Valid = true
	return json.Unmarshal(b, &o.V)
}

// EligibilityInput is the location rule an organiser sets (platform/eligibility).
type EligibilityInput struct {
	Type            string `json:"type"`
	Scope           string `json:"scope"`
	DistrictID      int64  `json:"districtId"`
	StateID         int64  `json:"stateId"`
	RequireVerified *bool  `json:"requireVerified"`
}

// Input is EventInput (docs/api.md). All fields optional for PUT.
type Input struct {
	SportID              Opt[int64]            `json:"sportId"`
	Title                Opt[string]           `json:"title"`
	Description          Opt[string]           `json:"description"`
	EventType            Opt[string]           `json:"eventType"`
	ParticipantType      Opt[string]           `json:"participantType"`
	TeamSizeMin          Opt[int]              `json:"teamSizeMin"`
	TeamSizeMax          Opt[int]              `json:"teamSizeMax"`
	CompetitionLevel     Opt[string]           `json:"competitionLevel"`
	Category             Opt[string]           `json:"category"`
	Eligibility          Opt[EligibilityInput] `json:"eligibility"`
	GenderCategory       Opt[string]           `json:"genderCategory"`
	AgeMin               Opt[int]              `json:"ageMin"`
	AgeMax               Opt[int]              `json:"ageMax"`
	VenueName            Opt[string]           `json:"venueName"`
	AddressLine          Opt[string]           `json:"addressLine"`
	City                 Opt[string]           `json:"city"`
	District             Opt[string]           `json:"district"`
	State                Opt[string]           `json:"state"`
	Pincode              Opt[string]           `json:"pincode"`
	Latitude             Opt[float64]          `json:"latitude"`
	Longitude            Opt[float64]          `json:"longitude"`
	StartsAt             Opt[time.Time]        `json:"startsAt"`
	EndsAt               Opt[time.Time]        `json:"endsAt"`
	RegistrationDeadline Opt[time.Time]        `json:"registrationDeadline"`
	EntryFeePaise        Opt[int64]            `json:"entryFeePaise"`
	PaymentModes         Opt[[]string]         `json:"paymentModes"`
	PaymentInstructions  Opt[string]           `json:"paymentInstructions"`
	MaxSlots             Opt[int]              `json:"maxSlots"`
	Rules                Opt[string]           `json:"rules"`
	PrizeInfo            Opt[string]           `json:"prizeInfo"`
	ContactName          Opt[string]           `json:"contactName"`
	ContactPhone         Opt[string]           `json:"contactPhone"`
	BannerUploadID       Opt[int64]            `json:"bannerUploadId"`
}

// ReasonInput is {reason} for cancel / reject / remove.
type ReasonInput struct {
	Reason string `json:"reason"`
}

// SportRef is {id, code, name}.
type SportRef struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// State is {id, code, name, isUT}.
type State struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
	IsUT bool   `json:"isUT"`
}

// District is {id, stateId, name}.
type District struct {
	ID      int64  `json:"id"`
	StateID int64  `json:"stateId"`
	Name    string `json:"name"`
}

// EligibilityRule is the event's location rule as stored, with names.
type EligibilityRule struct {
	Type            string    `json:"type"`
	Scope           *string   `json:"scope"`
	District        *District `json:"district"`
	State           *State    `json:"state"`
	RequireVerified bool      `json:"requireVerified"`
	Summary         string    `json:"summary"`
}

// MyEligibility is the caller's location eligibility for one event.
type MyEligibility struct {
	Eligible bool    `json:"eligible"`
	Via      *string `json:"via"`
	Pending  bool    `json:"pending"`
	Tag      *string `json:"tag"`
	Reason   *string `json:"reason"`
	Action   string  `json:"action"`
}

// EligibilityPreview is the organiser form preview line.
type EligibilityPreview struct {
	Valid         bool              `json:"valid"`
	Fields        map[string]string `json:"fields"`
	Summary       string            `json:"summary"`
	SuggestedType string            `json:"suggestedType"`
}

// Summary is EventSummary.
type Summary struct {
	ID                   int64            `json:"id"`
	Title                string           `json:"title"`
	Sport                SportRef         `json:"sport"`
	EventType            string           `json:"eventType"`
	ParticipantType      string           `json:"participantType"`
	CompetitionLevel     string           `json:"competitionLevel"`
	GenderCategory       string           `json:"genderCategory"`
	AgeMin               *int             `json:"ageMin"`
	AgeMax               *int             `json:"ageMax"`
	VenueName            string           `json:"venueName"`
	City                 string           `json:"city"`
	District             *string          `json:"district"`
	State                string           `json:"state"`
	StartsAt             time.Time        `json:"startsAt"`
	EndsAt               time.Time        `json:"endsAt"`
	RegistrationDeadline time.Time        `json:"registrationDeadline"`
	EntryFeePaise        int64            `json:"entryFeePaise"`
	MaxSlots             int              `json:"maxSlots"`
	RegistrationsCount   int              `json:"registrationsCount"`
	SlotsLeft            int              `json:"slotsLeft"`
	BannerURL            *string          `json:"bannerUrl"`
	Status               string           `json:"status"`
	IsFeatured           bool             `json:"isFeatured"`
	Organiser            *userdir.Summary `json:"organiser"`
	Category             string           `json:"category"`
	Eligibility          EligibilityRule  `json:"eligibility"`
	MyEligibility        *MyEligibility   `json:"myEligibility"`
	DistanceKm           *float64         `json:"distanceKm"`
}

// Event is the detail shape (EventSummary + more).
type Event struct {
	Summary
	Description         *string       `json:"description"`
	TeamSizeMin         *int          `json:"teamSizeMin"`
	TeamSizeMax         *int          `json:"teamSizeMax"`
	AddressLine         *string       `json:"addressLine"`
	Pincode             *string       `json:"pincode"`
	Latitude            *float64      `json:"latitude"`
	Longitude           *float64      `json:"longitude"`
	PaymentModes        []string      `json:"paymentModes"`
	PaymentInstructions *string       `json:"paymentInstructions"`
	Rules               *string       `json:"rules"`
	PrizeInfo           *string       `json:"prizeInfo"`
	ContactName         string        `json:"contactName"`
	ContactPhoneMasked  string        `json:"contactPhoneMasked"`
	RejectionReason     *string       `json:"rejectionReason"`
	CancelReason        *string       `json:"cancelReason"`
	PublishedAt         *time.Time    `json:"publishedAt"`
	IsOwner             bool          `json:"isOwner"`
	MyRegistration      *Registration `json:"myRegistration"`
	HasGroupChat        bool          `json:"hasGroupChat"`
	Listing             *ListingRef   `json:"listing"`
}

// AdminEvent is Event + listingOrders (GET /admin/events/:id).
type AdminEvent struct {
	Event
	ListingOrders []ListingOrder `json:"listingOrders"`
}

// AdminSummary is EventSummary + listing (GET /admin/events).
type AdminSummary struct {
	Summary
	Listing *ListingRef `json:"listing"`
}

// ListingRef is the owner-only listing summary on an event.
type ListingRef struct {
	OrderID    int64  `json:"orderId"`
	Status     string `json:"status"`
	TotalPaise int64  `json:"totalPaise"`
}

// TeamMember is one entry of a registration's team.
type TeamMember struct {
	Name string  `json:"name"`
	Role *string `json:"role"`
}

// Registration is the caller's registration on an event (read-only here; owned by registration).
type Registration struct {
	ID            int64        `json:"id"`
	EventID       int64        `json:"eventId"`
	Status        string       `json:"status"`
	PaymentStatus string       `json:"paymentStatus"`
	PaymentNote   *string      `json:"paymentNote"`
	TeamName      *string      `json:"teamName"`
	TeamMembers   []TeamMember `json:"teamMembers"`
	CreatedAt     time.Time    `json:"createdAt"`
	Event         *Summary     `json:"event"`

	QualifiedVia         string     `json:"qualifiedVia"`
	QualifiedDistrict    *District  `json:"qualifiedDistrict"`
	EligibilityFlag      string     `json:"eligibilityFlag"`
	EligibilityFlaggedAt *time.Time `json:"eligibilityFlaggedAt"`

	qualifiedDistrictID *int64
}

// Contact is EventContact (full number).
type Contact struct {
	ContactName         string   `json:"contactName"`
	ContactPhone        string   `json:"contactPhone"`
	PaymentModes        []string `json:"paymentModes"`
	PaymentInstructions *string  `json:"paymentInstructions"`
}

// ListingOrder is the admin view of a listing order (read-only here; owned by listingpayment).
type ListingOrder struct {
	OrderID         int64      `json:"orderId"`
	EventID         int64      `json:"eventId"`
	Status          string     `json:"status"`
	Quote           OrderQuote `json:"quote"`
	Provider        string     `json:"provider"`
	ProviderOrderID *string    `json:"providerOrderId"`
	RefundPaise     int64      `json:"refundPaise"`
	ExpiresAt       time.Time  `json:"expiresAt"`
	PaidAt          *time.Time `json:"paidAt"`
	EventStatus     string     `json:"eventStatus"`
}

// OrderQuote is the fee snapshot stored on a listing order.
type OrderQuote struct {
	FeeRuleID       int64  `json:"feeRuleId"`
	RuleName        string `json:"ruleName"`
	EntryFeePaise   int64  `json:"entryFeePaise"`
	MaxSlots        int64  `json:"maxSlots"`
	GrossEntryPaise int64  `json:"grossEntryPaise"`
	PercentBp       int64  `json:"percentBp"`
	MinFeePaise     int64  `json:"minFeePaise"`
	MaxFeePaise     *int64 `json:"maxFeePaise"`
	IsFreeEvent     bool   `json:"isFreeEvent"`
	BaseFeePaise    int64  `json:"baseFeePaise"`
	GstBp           int64  `json:"gstBp"`
	GstPaise        int64  `json:"gstPaise"`
	TotalPaise      int64  `json:"totalPaise"`
}

// rec is one events row (plus the sport join).
type rec struct {
	ID                  int64
	OrganiserID         int64
	SportID             int64
	SportCode           string
	SportName           string
	Title               string
	Description         *string
	EventType           string
	ParticipantType     string
	TeamSizeMin         *int
	TeamSizeMax         *int
	CompetitionLevel    string
	Category            string
	ElType              string
	ElScope             *string
	ElDistrictID        *int64
	ElStateID           *int64
	RequireVerified     bool
	GenderCategory      string
	AgeMin              *int
	AgeMax              *int
	VenueName           string
	AddressLine         *string
	City                string
	District            *string
	State               string
	Pincode             *string
	Latitude            *float64
	Longitude           *float64
	StartsAt            time.Time
	EndsAt              time.Time
	Deadline            time.Time
	EntryFeePaise       int64
	PaymentModes        []string
	PaymentInstructions *string
	MaxSlots            int
	Rules               *string
	PrizeInfo           *string
	ContactName         string
	PhoneLast4          string
	BannerUploadID      *int64
	Status              string
	RejectionReason     *string
	CancelReason        *string
	RegistrationsCount  int
	FeaturedAt          *time.Time
	PublishedAt         *time.Time
	CreatedAt           time.Time

	// dist is the haversine distance in km (only when the query had lat/lng).
	dist *float64
	// phone is the new plain contact number (10 digits) when it changed; never logged.
	phone string
}

// ListFilter holds GET /events filters.
type ListFilter struct {
	Q                string
	SportID          int64
	EventType        string
	CompetitionLevel string
	GenderCategory   string
	State            string
	District         string
	City             string
	From             *time.Time
	To               *time.Time
	MaxEntryFeePaise *int64
	EligibleForMe    bool
	IncludeClosed    bool
	Category         string
	ShowAll          bool
	Lat, Lng         *float64
	RadiusKm         int
	SortDistance     bool
}

// AdminFilter holds GET /admin/events filters.
type AdminFilter struct {
	Status  string
	SportID int64
	Q       string
}

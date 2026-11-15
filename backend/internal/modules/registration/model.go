package registration

import (
	"encoding/json"
	"time"

	"athlix/backend/internal/modules/event"
	"athlix/backend/internal/platform/userdir"
)

// Seed values of the settings this module reads (docs/config-system.md). Go default = seed.
const (
	defWithdrawUntilHours = 24
	defWaitlistEnabled    = true
	defFlagOnChange       = true
	defPageSize           = 20
)

// Contract limits (docs/api.md, Registrations).
const (
	maxTeamMembers = 30
	maxNameLen     = 80
	maxRoleLen     = 40
	maxNoteLen     = 200
)

// batchSize bounds every handler / job batch (CLAUDE.md: ≤ 500 rows).
const batchSize = 500

// promoteScan bounds how many waitlisted rows one promotion looks at.
const promoteScan = 50

// Registration statuses.
const (
	stRegistered = "registered"
	stWaitlisted = "waitlisted"
	stWithdrawn  = "withdrawn"
	stRejected   = "rejected"
)

// Eligibility flags (event_registrations.eligibility_flag).
const (
	flagNone     = "none"
	flagExpired  = "identity_expired"
	flagRejected = "identity_rejected"
	flagRevoked  = "identity_revoked"
	flagChanged  = "identity_changed"
)

var (
	statuses        = map[string]bool{stRegistered: true, stWaitlisted: true, stWithdrawn: true, stRejected: true}
	paymentStatuses = map[string]bool{"unpaid": true, "paid_online": true, "paid_offline": true, "waived": true}
)

// ist is India Standard Time.
var ist = time.FixedZone("IST", 5*3600+1800)

// TeamMember is one name on a team (names only; never verified).
type TeamMember struct {
	Name string  `json:"name"`
	Role *string `json:"role"`
}

// RegisterInput is POST /events/:id/registrations.
type RegisterInput struct {
	TeamName    *string      `json:"teamName"`
	TeamMembers []TeamMember `json:"teamMembers"`
}

// PatchInput is PATCH /events/:id/registrations/:regId (organiser).
type PatchInput struct {
	PaymentStatus *string `json:"paymentStatus"`
	PaymentNote   *string `json:"paymentNote"`
	Status        *string `json:"status"`
}

// District is {id, stateId, name}.
type District struct {
	ID      int64  `json:"id"`
	StateID int64  `json:"stateId"`
	Name    string `json:"name"`
}

// Base holds the fields shared by Registration and RegistrationForOrganiser.
type Base struct {
	ID                   int64        `json:"id"`
	EventID              int64        `json:"eventId"`
	Status               string       `json:"status"`
	PaymentStatus        string       `json:"paymentStatus"`
	PaymentNote          *string      `json:"paymentNote"`
	TeamName             *string      `json:"teamName"`
	TeamMembers          []TeamMember `json:"teamMembers"`
	CreatedAt            time.Time    `json:"createdAt"`
	QualifiedVia         string       `json:"qualifiedVia"`
	QualifiedDistrict    *District    `json:"qualifiedDistrict"`
	EligibilityFlag      string       `json:"eligibilityFlag"`
	EligibilityFlaggedAt *time.Time   `json:"eligibilityFlaggedAt"`
}

// Registration is the player's view.
type Registration struct {
	Base
	Event *event.Summary `json:"event"`
}

// ForOrganiser is RegistrationForOrganiser (no event, plus user and qualifiedLabel).
type ForOrganiser struct {
	Base
	User           *userdir.Summary `json:"user"`
	QualifiedLabel string           `json:"qualifiedLabel"`
}

// rec is one event_registrations row.
type rec struct {
	ID                   int64
	EventID              int64
	UserID               int64
	TeamName             *string
	TeamMembers          []TeamMember
	Status               string
	PaymentStatus        string
	PaymentNote          *string
	CreatedAt            time.Time
	QualifiedVia         string
	QualifiedIdentityID  *int64
	QualifiedDistrictID  *int64
	EligibilityFlag      string
	EligibilityFlaggedAt *time.Time
}

func (r rec) membersJSON() string {
	if r.TeamMembers == nil {
		return "[]"
	}
	b, _ := json.Marshal(r.TeamMembers)
	return string(b)
}

// evRec is the part of an events row registration needs.
type evRec struct {
	ID                 int64
	OrganiserID        int64
	Status             string
	StartsAt           time.Time
	Deadline           time.Time
	MaxSlots           int
	RegistrationsCount int
	GenderCategory     string
	AgeMin             *int
	AgeMax             *int
	ElType             string
	ElScope            *string
	ElDistrictID       *int64
	ElStateID          *int64
	RequireVerified    bool
}

// flagged is one registration.eligibility_flagged to publish after commit.
type flagged struct {
	reg         rec
	flag        string
	organiserID int64
}

// outbox collects the events of one transaction; they are published after commit.
type outbox struct {
	organiserID int64
	created     []rec
	withdrawn   []rec
	promoted    []rec
	paid        []rec
	flagged     []flagged
}

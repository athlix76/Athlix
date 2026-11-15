package consultation

import (
	"encoding/json"
	"time"

	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/userdir"
)

// EventPayload is what consultation.requested / consultation.responded carry: the shared
// events.ConsultationPayload plus responderId (the coach's user id) on consultation.responded,
// so consumers do not have to rely on the event's actorId. The requester is always playerId.
type EventPayload struct {
	events.ConsultationPayload
}

// Modes is the closed list of consultation modes.
var Modes = []string{"online", "in_person"}

// Statuses is the closed list of request statuses (consultation_requests.status).
var Statuses = []string{"pending", "accepted", "declined", "cancelled", "completed"}

const (
	maxDescription = 1000
	maxFeeNote     = 200
	maxMessage     = 1000
)

// Listing is the response of GET /coaches/:userId/consultation and PUT /me/consultation.
type Listing struct {
	Coach       userdir.Summary `json:"coach"`
	Offered     bool            `json:"offered"`
	Description *string         `json:"description"`
	FeeNote     *string         `json:"feeNote"`
	Modes       []string        `json:"modes"`
	CanRequest  bool            `json:"canRequest"`
}

// Request is the ConsultationRequest shape in docs/api.md.
type Request struct {
	ID            int64           `json:"id"`
	Coach         userdir.Summary `json:"coach"`
	Player        userdir.Summary `json:"player"`
	Message       string          `json:"message"`
	PreferredMode string          `json:"preferredMode"`
	Status        string          `json:"status"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

// listingRow is one coach_consultations row.
type listingRow struct {
	Offered     bool
	Description *string
	FeeNote     *string
	Modes       []string
}

// requestRow is one consultation_requests row.
type requestRow struct {
	ID            int64
	CoachID       int64
	PlayerID      int64
	Message       string
	PreferredMode string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Opt is a JSON field that tells "missing" (Set=false) from "null" (Set, !Valid).
type Opt[T any] struct {
	Set   bool
	Valid bool
	V     T
}

// UnmarshalJSON implements json.Unmarshaler. It is only called when the key is present.
func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Valid = false
		return nil
	}
	o.Valid = true
	return json.Unmarshal(b, &o.V)
}

// ListingInput is the body of PUT /me/consultation (missing key = unchanged).
type ListingInput struct {
	Offered     *bool       `json:"offered"`
	Description Opt[string] `json:"description"`
	FeeNote     Opt[string] `json:"feeNote"`
	Modes       *[]string   `json:"modes"`
}

// RequestInput is the body of POST /coaches/:userId/consultation-requests.
type RequestInput struct {
	Message       string `json:"message"`
	PreferredMode string `json:"preferredMode"`
}

// RespondInput is the body of POST /consultation-requests/:id/respond. The note has no
// column in consultation_requests and is not stored.
type RespondInput struct {
	Accept *bool   `json:"accept"`
	Note   *string `json:"note"`
}

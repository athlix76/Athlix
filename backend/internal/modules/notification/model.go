package notification

import (
	"encoding/json"
	"time"
)

// Notification types that are not domain event names.
const (
	TypeEventReminder = "event.reminder"
	TypeBroadcast     = "broadcast"
)

// EventBroadcastPush is an internal event this module publishes to itself so broadcast pushes
// are sent from the worker in batches, not inside the admin request.
const EventBroadcastPush = "notification.broadcast_push"

// Notification categories (user_settings.notification_prefs keys). An empty category means
// an account notice that is always pushed.
const (
	CategoryMessages  = "messages"
	CategoryEvents    = "events"
	CategorySocial    = "social"
	CategoryMarketing = "marketing"
	categoryAccount   = ""
)

// defaultPrefs apply when the user has no settings row or a key is missing.
var defaultPrefs = map[string]bool{CategoryMessages: true, CategoryEvents: true, CategorySocial: true, CategoryMarketing: false}

const (
	listDefaultLimit = 20
	readMaxIDs       = 100
	batchSize        = 500
	maxTokensPerUser = 10
	reminderMaxBatch = 20 // batches of batchSize per job run
	dedupeTTL        = 72 * time.Hour
	titleMaxLen      = 120
	bodyMaxLen       = 500
	defaultReminderH = 24 // seed value of notification.event_reminder_hours
)

// Notification is the API shape (docs/api.md "Notifications").
type Notification struct {
	ID        int64           `json:"id"`
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Data      json.RawMessage `json:"data"`
	ReadAt    *time.Time      `json:"readAt"`
	CreatedAt time.Time       `json:"createdAt"`
}

type deviceInput struct {
	Platform   string  `json:"platform"`
	Token      string  `json:"token"`
	AppVersion *string `json:"appVersion"`
}

type deviceDeleteInput struct {
	Token string `json:"token"`
}

type readInput struct {
	IDs []int64 `json:"ids"`
	All bool    `json:"all"`
}

// BroadcastInput is the admin broadcast body.
type BroadcastInput struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Audience Audience `json:"audience"`
}

// Audience filters broadcast recipients.
type Audience struct {
	Roles         []string `json:"roles"`
	SportID       int64    `json:"sportId"`
	State         string   `json:"state"`
	IncludeMinors bool     `json:"includeMinors"`
}

// BroadcastResult is the admin broadcast response.
type BroadcastResult struct {
	Queued int64 `json:"queued"`
}

// broadcastPushPayload drives the batched push of one broadcast: rows with
// AfterID < id <= LastID and type 'broadcast'.
type broadcastPushPayload struct {
	FirstID int64 `json:"firstId"`
	LastID  int64 `json:"lastId"`
	AfterID int64 `json:"afterId"`
}

// note is one notification to write and/or push.
type note struct {
	Type     string
	Title    string
	Body     string
	Data     map[string]int64  // deep-link ids
	Links    map[string]string // deep-link strings ({"screen": "location_identity"}, rejectionCode)
	Category string
}

func (n note) dataJSON() string {
	if len(n.Data) == 0 && len(n.Links) == 0 {
		return "{}"
	}
	all := make(map[string]any, len(n.Data)+len(n.Links))
	for k, v := range n.Data {
		all[k] = v
	}
	for k, v := range n.Links {
		all[k] = v
	}
	b, _ := json.Marshal(all)
	return string(b)
}

type device struct {
	ID     int64
	UserID int64
	Token  string
}

type reminderRow struct {
	RegID    int64
	EventID  int64
	UserID   int64
	Title    string
	StartsAt time.Time
}

type broadcastRow struct {
	ID     int64
	UserID int64
	Title  string
	Body   string
}

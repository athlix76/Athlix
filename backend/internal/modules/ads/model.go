package ads

import (
	"encoding/json"
	"time"
)

// Placements is the closed list of ad slots (ad_campaigns.placement).
var Placements = []string{"home_feed", "events_list", "search", "event_detail"}

// Statuses is the closed list of campaign statuses (ad_campaigns.status).
var Statuses = []string{"draft", "active", "paused", "ended"}

// Label is the disclosure shown on every served ad.
const Label = "Sponsored"

// maxServedCeiling bounds the ads.max_per_request setting (default 3).
const maxServedCeiling = 10

// Ad is the app-facing shape (docs/api.md "Ads").
type Ad struct {
	ID             int64   `json:"id"`
	AdvertiserName string  `json:"advertiserName"`
	Title          string  `json:"title"`
	Body           *string `json:"body"`
	ImageURL       *string `json:"imageUrl"`
	TargetURL      string  `json:"targetUrl"`
	Placement      string  `json:"placement"`
	Label          string  `json:"label"`
}

// Campaign is the admin shape (AdCampaign in docs/api.md).
type Campaign struct {
	ID             int64     `json:"id"`
	AdvertiserName string    `json:"advertiserName"`
	Title          string    `json:"title"`
	Body           *string   `json:"body"`
	ImageUploadID  *int64    `json:"imageUploadId"`
	ImageURL       *string   `json:"imageUrl"`
	TargetURL      string    `json:"targetUrl"`
	Placement      string    `json:"placement"`
	SportID        *int64    `json:"sportId"`
	State          *string   `json:"state"`
	Priority       int       `json:"priority"`
	StartsAt       time.Time `json:"startsAt"`
	EndsAt         time.Time `json:"endsAt"`
	Status         string    `json:"status"`
}

// DayStat is one row of GET /admin/ads/:id/stats.
type DayStat struct {
	Date        string `json:"date"`
	Impressions int64  `json:"impressions"`
	Clicks      int64  `json:"clicks"`
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

// CampaignInput is the body of POST (all required fields) and PUT (partial) /admin/ads.
type CampaignInput struct {
	AdvertiserName *string     `json:"advertiserName"`
	Title          *string     `json:"title"`
	Body           Opt[string] `json:"body"`
	ImageUploadID  Opt[int64]  `json:"imageUploadId"`
	TargetURL      *string     `json:"targetUrl"`
	Placement      *string     `json:"placement"`
	SportID        Opt[int64]  `json:"sportId"`
	State          Opt[string] `json:"state"`
	Priority       *int        `json:"priority"`
	StartsAt       *time.Time  `json:"startsAt"`
	EndsAt         *time.Time  `json:"endsAt"`
	Status         *string     `json:"status"`
}

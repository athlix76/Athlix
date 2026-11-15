package search

import (
	"time"

	"athlix/backend/internal/platform/userdir"
)

// Defaults equal the seed values in migrations/0007_seed.sql.
const (
	defaultPageSize = 20 // search.page_size
	defaultMinors   = true
	globalMax       = 5 // GET /search returns at most 5 of each kind
	minQueryLen     = 2
	maxQueryLen     = 100
	maxPlaceLen     = 64
	maxAge          = 120
)

var roles = map[string]bool{"player": true, "coach": true, "organiser": true}

var achievementLevels = map[string]bool{
	"school": true, "college": true, "club": true, "district": true,
	"state": true, "national": true, "international": true, "other": true,
}

// ist is used for "current year" in age filters (the app serves India).
var ist = time.FixedZone("IST", 5*3600+1800)

// SportRef is {id, code, name} (docs/api.md common objects).
type SportRef struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// PersonResult = UserSummary + bio + followersCount.
type PersonResult struct {
	userdir.Summary
	Bio            *string `json:"bio"`
	FollowersCount int     `json:"followersCount"`
}

// EventSummary mirrors docs/api.md "EventSummary".
type EventSummary struct {
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
	StartsAt             string           `json:"startsAt"`
	EndsAt               string           `json:"endsAt"`
	RegistrationDeadline string           `json:"registrationDeadline"`
	EntryFeePaise        int64            `json:"entryFeePaise"`
	MaxSlots             int              `json:"maxSlots"`
	RegistrationsCount   int              `json:"registrationsCount"`
	SlotsLeft            int              `json:"slotsLeft"`
	BannerURL            *string          `json:"bannerUrl"`
	Status               string           `json:"status"`
	IsFeatured           bool             `json:"isFeatured"`
	Organiser            *userdir.Summary `json:"organiser"`
}

// Community mirrors docs/api.md "Community".
type Community struct {
	ID          int64            `json:"id"`
	Title       string           `json:"title"`
	Description *string          `json:"description"`
	AvatarURL   *string          `json:"avatarUrl"`
	Sport       *SportRef        `json:"sport"`
	City        *string          `json:"city"`
	State       *string          `json:"state"`
	Visibility  string           `json:"visibility"`
	JoinPolicy  string           `json:"joinPolicy"`
	MemberCount int              `json:"memberCount"`
	Owner       *userdir.Summary `json:"owner"`
	MyStatus    string           `json:"myStatus"`
	MyRole      *string          `json:"myRole"`
	CreatedAt   string           `json:"createdAt"`
}

// GlobalResult is the response of GET /search.
type GlobalResult struct {
	People      []userdir.Summary `json:"people"`
	Events      []EventSummary    `json:"events"`
	Communities []Community       `json:"communities"`
}

// PeopleFilter is the query of GET /search/people.
type PeopleFilter struct {
	Q                string
	Role             string
	SportID          int64
	PositionID       int64
	State            string
	District         string
	City             string
	AgeMin           *int
	AgeMax           *int
	AchievementLevel string
}

// advanced reports whether a filter needs the advanced_search_filters benefit.
func (f PeopleFilter) advanced() bool {
	return f.AgeMin != nil || f.AgeMax != nil || f.AchievementLevel != ""
}

type personRow struct {
	ID             int64
	Bio            *string
	FollowersCount int
	IsMinor        bool
	Boosted        bool
}

type eventRow struct {
	EventSummary
	OrganiserID int64
	BannerID    *int64
}

type communityRow struct {
	Community
	OwnerID  *int64
	AvatarID *int64
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

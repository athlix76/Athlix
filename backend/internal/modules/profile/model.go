package profile

import (
	"database/sql"
	"time"

	"athlix/backend/internal/platform/userdir"
)

// Visibilities are the allowed profiles.visibility / stats_visibility values.
var Visibilities = []string{"public", "followers", "private"}

// Levels are the allowed achievements.level values.
var Levels = []string{"school", "college", "club", "district", "state", "national", "international", "other"}

// SportRef is the SportRef JSON in docs/api.md.
type SportRef struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// ProfileSport is one entry of Profile.sports.
type ProfileSport struct {
	Sport     SportRef  `json:"sport"`
	Position  *SportRef `json:"position"`
	IsPrimary bool      `json:"isPrimary"`
}

// Consultation is Profile.consultation (coaches only).
type Consultation struct {
	Offered bool `json:"offered"`
}

// OrganiserInfo is Profile.organiser (organisers only).
type OrganiserInfo struct {
	Status  string  `json:"status"`
	OrgName *string `json:"orgName"`
	OrgType *string `json:"orgType"`
}

// Profile is the Profile JSON in docs/api.md.
type Profile struct {
	User              userdir.Summary `json:"user"`
	IsOwn             bool            `json:"isOwn"`
	Restricted        bool            `json:"restricted"`
	Bio               *string         `json:"bio"`
	Sports            []ProfileSport  `json:"sports"`
	District          *string         `json:"district"`
	AcademyClub       *string         `json:"academyClub"`
	BirthYear         *int            `json:"birthYear"`
	Visibility        *string         `json:"visibility"`
	StatsVisibility   *string         `json:"statsVisibility"`
	FollowersCount    int             `json:"followersCount"`
	FollowingCount    int             `json:"followingCount"`
	IsFollowing       bool            `json:"isFollowing"`
	IsFollowedBy      bool            `json:"isFollowedBy"`
	IsBlockedByMe     bool            `json:"isBlockedByMe"`
	CanMessage        bool            `json:"canMessage"`
	Badges            []string        `json:"badges"`
	CoachingSinceYear *int            `json:"coachingSinceYear"`
	Consultation      *Consultation   `json:"consultation"`
	Organiser         *OrganiserInfo  `json:"organiser"`
}

// setupBody is ProfileSetup (POST /onboarding/profile).
type setupBody struct {
	DisplayName   string  `json:"displayName"`
	SportID       int64   `json:"sportId"`
	PositionID    *int64  `json:"positionId"`
	City          *string `json:"city"`
	District      *string `json:"district"`
	State         *string `json:"state"`
	Bio           *string `json:"bio"`
	AcademyClub   *string `json:"academyClub"`
	PhotoUploadID *int64  `json:"photoUploadId"`
}

// sportInput is one entry of ProfileUpdate.sports.
type sportInput struct {
	SportID    int64  `json:"sportId"`
	PositionID *int64 `json:"positionId"`
	IsPrimary  bool   `json:"isPrimary"`
}

// updateBody is ProfileUpdate (PUT /me/profile); a missing field means unchanged and an
// empty string clears an optional text field.
type updateBody struct {
	DisplayName       *string       `json:"displayName"`
	Bio               *string       `json:"bio"`
	Sports            *[]sportInput `json:"sports"`
	City              *string       `json:"city"`
	District          *string       `json:"district"`
	State             *string       `json:"state"`
	AcademyClub       *string       `json:"academyClub"`
	PhotoUploadID     *int64        `json:"photoUploadId"`
	Visibility        *string       `json:"visibility"`
	StatsVisibility   *string       `json:"statsVisibility"`
	CoachingSinceYear *int          `json:"coachingSinceYear"`
}

// Viewer is one row of GET /me/profile-viewers.
type Viewer struct {
	User     userdir.Summary `json:"user"`
	ViewedOn string          `json:"viewedOn"`
}

// StatEntry is the StatEntry JSON.
type StatEntry struct {
	ID         int64          `json:"id"`
	SportID    int64          `json:"sportId"`
	TemplateID int64          `json:"templateId"`
	Season     string         `json:"season"`
	Stats      map[string]any `json:"stats"`
	UpdatedAt  time.Time      `json:"updatedAt"`
}

// Stats is GET /profiles/:userId/stats.
type Stats struct {
	Entries []StatEntry        `json:"entries"`
	Totals  map[string]float64 `json:"totals"`
}

type statCreateBody struct {
	SportID    int64          `json:"sportId"`
	TemplateID int64          `json:"templateId"`
	Season     string         `json:"season"`
	Stats      map[string]any `json:"stats"`
}

type statUpdateBody struct {
	Season *string         `json:"season"`
	Stats  *map[string]any `json:"stats"`
}

// Achievement is the Achievement JSON.
type Achievement struct {
	ID                  int64     `json:"id"`
	SportID             *int64    `json:"sportId"`
	Title               string    `json:"title"`
	Description         *string   `json:"description"`
	Level               string    `json:"level"`
	AchievedOn          *string   `json:"achievedOn"`
	CertificateUploadID *int64    `json:"certificateUploadId"`
	CertificateURL      *string   `json:"certificateUrl"`
	CreatedAt           time.Time `json:"createdAt"`
}

// achievementBody is AchievementInput (partial on PUT).
type achievementBody struct {
	SportID             *int64  `json:"sportId"`
	Title               *string `json:"title"`
	Description         *string `json:"description"`
	Level               *string `json:"level"`
	AchievedOn          *string `json:"achievedOn"`
	CertificateUploadID *int64  `json:"certificateUploadId"`
}

// row is a profiles row.
type row struct {
	Bio               sql.NullString
	SportID           sql.NullInt64
	PositionID        sql.NullInt64
	City              sql.NullString
	District          sql.NullString
	State             sql.NullString
	AcademyClub       sql.NullString
	PhotoUploadID     sql.NullInt64
	BirthYear         sql.NullInt64
	Visibility        string
	StatsVisibility   string
	CoachingSinceYear sql.NullInt64
	FollowersCount    int
	FollowingCount    int
}

type statRow struct {
	ID         int64
	SportID    int64
	TemplateID int64
	Season     string
	Stats      []byte
	UpdatedAt  time.Time
}

type achievementRow struct {
	ID          int64
	SportID     sql.NullInt64
	Title       string
	Description sql.NullString
	Level       string
	AchievedOn  sql.NullTime
	CertID      sql.NullInt64
	CreatedAt   time.Time
}

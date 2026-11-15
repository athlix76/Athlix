package moderation

import (
	"time"

	"athlix/backend/internal/platform/userdir"
)

// Target types (reports.target_type, moderation_actions.target_type).
const (
	TargetUser         = "user"
	TargetEvent        = "event"
	TargetPost         = "post"
	TargetComment      = "comment"
	TargetMessage      = "message"
	TargetConversation = "conversation"
	TargetCommunity    = "community" // a conversation of type 'community' (Community.id = conversation id)
)

// Actions (moderation_actions.action). "dismiss" exists only on report resolution.
const (
	ActionDismiss   = "dismiss"
	ActionHide      = "hide"
	ActionUnhide    = "unhide"
	ActionRemove    = "remove"
	ActionSuspend   = "suspend_user"
	ActionUnsuspend = "unsuspend_user"
	ActionWarn      = "warn"
	ActionAutoHide  = "auto_hide"
)

// Report statuses.
const (
	StatusOpen      = "open"
	StatusActioned  = "actioned"
	StatusDismissed = "dismissed"
)

// ReasonUnderageSafety is shown first in the admin queue (docs/security.md).
const ReasonUnderageSafety = "underage_safety"

// Defaults equal the seed values in migrations/0007_seed.sql and 0011_events_ads_moderation.sql.
const (
	defaultAutoHideThreshold = 5
	defaultReportRateLimit   = 20 // moderation.report_rate_limit_per_hour (0011): report:<userId> per hour
	reportRateWindow         = time.Hour
	maxDetailsLen            = 1000
	maxNoteLen               = 500
	previewLen               = 200
	defaultPageSize          = 20
)

var targetTypes = map[string]bool{
	TargetUser: true, TargetEvent: true, TargetPost: true,
	TargetComment: true, TargetMessage: true, TargetConversation: true, TargetCommunity: true,
}

// targetTypeList is the closed list in error messages and preview order.
var targetTypeList = []string{TargetUser, TargetEvent, TargetPost, TargetComment, TargetMessage, TargetConversation, TargetCommunity}

var reasons = map[string]bool{
	"spam": true, "fake_profile": true, "harassment": true, "scam": true,
	"inappropriate": true, ReasonUnderageSafety: true, "payment_issue": true, "other": true,
}

var resolveActions = map[string]bool{
	ActionDismiss: true, ActionHide: true, ActionRemove: true, ActionSuspend: true, ActionWarn: true,
}

var directActions = map[string]bool{
	ActionHide: true, ActionUnhide: true, ActionRemove: true,
	ActionSuspend: true, ActionUnsuspend: true, ActionWarn: true,
}

var reportStatuses = map[string]bool{StatusOpen: true, StatusActioned: true, StatusDismissed: true}

// autoHideable target types have a reversible 'hidden' status value; the others are only
// queued. users (suspended), events and conversations / communities (removed) have no hidden
// state, and their only statuses are punitive for other people (registrants, members), so a
// group of reporters must not be able to trigger them (see README "Rules").
var autoHideable = map[string]bool{TargetPost: true, TargetComment: true, TargetMessage: true}

// CreateReportInput is the body of POST /reports.
type CreateReportInput struct {
	TargetType string  `json:"targetType"`
	TargetID   int64   `json:"targetId"`
	Reason     string  `json:"reason"`
	Details    *string `json:"details"`
}

// ReportRef is the response of POST /reports.
type ReportRef struct {
	ID int64 `json:"id"`
}

// TargetPreview is a short view of the reported item for the admin queue. ConversationID is
// set for message (its conversation), conversation and community targets, else null.
type TargetPreview struct {
	Title          *string `json:"title"`
	Text           *string `json:"text"`
	OwnerID        *int64  `json:"ownerId"`
	ConversationID *int64  `json:"conversationId"`
}

// AdminReport is one row of the admin reports queue (docs/api.md).
type AdminReport struct {
	ID                  int64            `json:"id"`
	Reporter            *userdir.Summary `json:"reporter"`
	TargetType          string           `json:"targetType"`
	TargetID            int64            `json:"targetId"`
	TargetPreview       TargetPreview    `json:"targetPreview"`
	Reason              string           `json:"reason"`
	Details             *string          `json:"details"`
	Status              string           `json:"status"`
	OpenReportsOnTarget int              `json:"openReportsOnTarget"`
	ResolutionNote      *string          `json:"resolutionNote"`
	CreatedAt           string           `json:"createdAt"`
	ResolvedAt          *string          `json:"resolvedAt"`
}

// AdminRef names the admin who acted (null = automatic).
type AdminRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ModerationAction is one moderation_actions row (docs/api.md).
type ModerationAction struct {
	ID         int64     `json:"id"`
	Admin      *AdminRef `json:"admin"`
	TargetType string    `json:"targetType"`
	TargetID   int64     `json:"targetId"`
	Action     string    `json:"action"`
	Reason     *string   `json:"reason"`
	ReportID   *int64    `json:"reportId"`
	CreatedAt  string    `json:"createdAt"`
}

// ResolveInput is the body of POST /admin/reports/:id/resolve.
type ResolveInput struct {
	Action string  `json:"action"`
	Note   *string `json:"note"`
}

// ActionInput is the body of POST /admin/moderation/actions.
type ActionInput struct {
	TargetType string `json:"targetType"`
	TargetID   int64  `json:"targetId"`
	Action     string `json:"action"`
	Reason     string `json:"reason"`
}

// ReportFilter is the query of GET /admin/reports.
type ReportFilter struct {
	Status     string
	TargetType string
	Reason     string
}

// reportRow is a reports row plus the open-report count on its target.
type reportRow struct {
	ID             int64
	ReporterID     int64
	TargetType     string
	TargetID       int64
	Reason         string
	Details        *string
	Status         string
	ResolutionNote *string
	CreatedAt      time.Time
	ResolvedAt     *time.Time
	OpenOnTarget   int
}

// actionRow is a moderation_actions row joined with the admin name.
type actionRow struct {
	ID         int64
	AdminID    *int64
	AdminName  *string
	TargetType string
	TargetID   int64
	Action     string
	Reason     *string
	ReportID   *int64
	CreatedAt  time.Time
}

// targetState is the current status and owner of a target row.
type targetState struct {
	Status  string
	OwnerID int64 // 0 = none (system message, direct conversation)
	SportID int64 // events only
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func tsPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := ts(*t)
	return &s
}

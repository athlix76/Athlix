package organiser

import (
	"time"

	"athlix/backend/internal/platform/userdir"
)

// Statuses of organiser_profiles.status.
const (
	StatusNotStarted = "not_started"
	StatusDraft      = "draft"
	StatusSubmitted  = "submitted"
	StatusApproved   = "approved"
	StatusRejected   = "rejected"
)

// Step input types (organiser_verification_steps.input_type).
const (
	inputText     = "text"
	inputDocument = "document"
	inputCheckbox = "checkbox"
)

// orgTypes is the organiser_profiles.org_type enum.
var orgTypes = []string{"individual", "club", "academy", "school", "college", "association", "company", "other"}

// Field limits match the column sizes in migrations/0002.
const (
	maxOrgName = 120
	maxValue   = 500
	maxNote    = 500
)

// Verification is the OrganiserVerification object of docs/api.md.
type Verification struct {
	Status      string     `json:"status"`
	OrgName     *string    `json:"orgName"`
	OrgType     *string    `json:"orgType"`
	ReviewNote  *string    `json:"reviewNote"`
	SubmittedAt *time.Time `json:"submittedAt"`
	ReviewedAt  *time.Time `json:"reviewedAt"`
	Steps       []Step     `json:"steps"`
}

// Step is one checklist step with the caller's submission (null when not filled).
type Step struct {
	StepID      int64       `json:"stepId"`
	StepCode    string      `json:"stepCode"`
	Title       string      `json:"title"`
	Description *string     `json:"description"`
	InputType   string      `json:"inputType"`
	IsRequired  bool        `json:"isRequired"`
	Submission  *Submission `json:"submission"`
}

// Submission is the stored value of a step.
type Submission struct {
	ValueText  *string `json:"valueText"`
	Document   *DocRef `json:"document"`
	Checked    *bool   `json:"checked"`
	Status     string  `json:"status"`
	ReviewNote *string `json:"reviewNote"`
}

// DocRef is an attached document; url is a short-lived signed link (null if unavailable).
type DocRef struct {
	UploadID int64   `json:"uploadId"`
	URL      *string `json:"url"`
}

// AdminVerification is the admin detail: OrganiserVerification + the user card.
type AdminVerification struct {
	User userdir.Summary `json:"user"`
	Verification
}

// AdminRow is one row of GET /admin/organisers.
type AdminRow struct {
	User        userdir.Summary `json:"user"`
	OrgName     *string         `json:"orgName"`
	OrgType     *string         `json:"orgType"`
	Status      string          `json:"status"`
	SubmittedAt *time.Time      `json:"submittedAt"`
}

// saveRequest is the PUT body. A missing field means unchanged.
type saveRequest struct {
	OrgName *string     `json:"orgName"`
	OrgType *string     `json:"orgType"`
	Steps   []stepInput `json:"steps"`
}

type stepInput struct {
	StepID           int64   `json:"stepId"`
	ValueText        *string `json:"valueText"`
	DocumentUploadID *int64  `json:"documentUploadId"`
	Checked          *bool   `json:"checked"`
}

type noteRequest struct {
	Note *string `json:"note"`
}

// profileRow is organiser_profiles.
type profileRow struct {
	exists      bool
	OrgName     *string
	OrgType     *string
	Status      string
	ReviewNote  *string
	SubmittedAt *time.Time
	ReviewedAt  *time.Time
}

// stepRow is a step joined with the user's submission.
type stepRow struct {
	ID          int64
	Code        string
	Title       string
	Description *string
	InputType   string
	Required    bool
	Active      bool
	HasSub      bool
	ValueText   *string
	UploadID    *int64
	SubStatus   string
	SubNote     *string
}

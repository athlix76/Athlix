package appconfig

import (
	"database/sql"
	"encoding/json"
	"time"

	"athlix/backend/internal/modules/sport"
	"athlix/backend/internal/platform/settings"
)

// DocTypes is the closed list of policy_documents.doc_type values (migrations 0006 + 0008).
var DocTypes = []string{
	"terms", "privacy", "listing_refund", "guardian_consent", "community_guidelines",
	"organiser_terms", "entry_fee_disclaimer", "location_identity_notice",
}

// Allowed values for the enum columns of verification_requirements and the sports tables.
var (
	ReqRoles        = []string{"player", "coach", "organiser"}
	ReqRequirements = []string{"digilocker", "guardian_consent_if_minor", "organiser_approval", "coach_pro_subscription"}
	ReqGates        = []string{"app_access", "create_event", "offer_consultation", "create_community"}
	StepInputTypes  = []string{"text", "document", "checkbox"}
	AppliesTo       = []string{"player", "coach", "both"}
)

// ConfigEntry is the ConfigEntry JSON in docs/api.md (same shape as settings.Entry).
type ConfigEntry = settings.Entry

// ConfigChange is one row of GET /admin/config/:key/history.
type ConfigChange struct {
	OldValue  json.RawMessage `json:"oldValue"`
	NewValue  json.RawMessage `json:"newValue"`
	ChangedBy AdminRef        `json:"changedBy"`
	ChangedAt time.Time       `json:"changedAt"`
}

// AdminRef is the {id, name} of an admin.
type AdminRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// PolicySummary is one entry of Bootstrap.policies.
type PolicySummary struct {
	ID                 int64  `json:"id"`
	DocType            string `json:"docType"`
	Version            int    `json:"version"`
	Title              string `json:"title"`
	RequiresAcceptance bool   `json:"requiresAcceptance"`
}

// PolicyDocument is the PolicyDocument JSON in docs/api.md.
type PolicyDocument struct {
	ID                 int64     `json:"id"`
	DocType            string    `json:"docType"`
	Version            int       `json:"version"`
	Title              string    `json:"title"`
	ContentMd          string    `json:"contentMd"`
	RequiresAcceptance bool      `json:"requiresAcceptance"`
	EffectiveFrom      time.Time `json:"effectiveFrom"`
}

// AdminPolicy is PolicyDocument + {active, createdAt} for the admin list.
type AdminPolicy struct {
	PolicyDocument
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
}

// MinVersion is Bootstrap.minVersion.
type MinVersion struct {
	Android string `json:"android"`
	IOS     string `json:"ios"`
}

// Bootstrap is GET /bootstrap.
type Bootstrap struct {
	Config      map[string]json.RawMessage `json:"config"`
	Policies    []PolicySummary            `json:"policies"`
	Sports      []sport.Sport              `json:"sports"`
	MinVersion  MinVersion                 `json:"minVersion"`
	Maintenance bool                       `json:"maintenance"`
	ServerTime  time.Time                  `json:"serverTime"`
}

// Requirement is a verification_requirements row.
type Requirement struct {
	ID          int64  `json:"id"`
	Role        string `json:"role"`
	Requirement string `json:"requirement"`
	IsRequired  bool   `json:"isRequired"`
	Gates       string `json:"gates"`
}

// OrganiserStep is an organiser_verification_steps row.
type OrganiserStep struct {
	ID          int64   `json:"id"`
	StepCode    string  `json:"stepCode"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	InputType   string  `json:"inputType"`
	IsRequired  bool    `json:"isRequired"`
	SortOrder   int     `json:"sortOrder"`
	Active      bool    `json:"active"`
}

// ---- request bodies ----

type putConfigBody struct {
	Value   json.RawMessage `json:"value"`
	Version int             `json:"version"`
}

type createPolicyBody struct {
	DocType            string `json:"docType"`
	Title              string `json:"title"`
	ContentMd          string `json:"contentMd"`
	RequiresAcceptance bool   `json:"requiresAcceptance"`
}

type createReqBody struct {
	Role        string `json:"role"`
	Requirement string `json:"requirement"`
	IsRequired  *bool  `json:"isRequired"`
	Gates       string `json:"gates"`
}

type updateReqBody struct {
	IsRequired *bool `json:"isRequired"`
}

type stepBody struct {
	StepCode    *string `json:"stepCode"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	InputType   *string `json:"inputType"`
	IsRequired  *bool   `json:"isRequired"`
	SortOrder   *int    `json:"sortOrder"`
	Active      *bool   `json:"active"`
}

type createSportBody struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	IsTeamSport bool   `json:"isTeamSport"`
	SortOrder   int    `json:"sortOrder"`
}

type updateSportBody struct {
	Name        *string `json:"name"`
	IsTeamSport *bool   `json:"isTeamSport"`
	SortOrder   *int    `json:"sortOrder"`
	Active      *bool   `json:"active"`
}

type createPositionBody struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	AppliesTo string `json:"appliesTo"`
	SortOrder int    `json:"sortOrder"`
}

type updatePositionBody struct {
	Name      *string `json:"name"`
	AppliesTo *string `json:"appliesTo"`
	SortOrder *int    `json:"sortOrder"`
	Active    *bool   `json:"active"`
}

type createTemplateBody struct {
	PositionID *int64            `json:"positionId"`
	Name       string            `json:"name"`
	Fields     []sport.StatField `json:"fields"`
}

type updateTemplateBody struct {
	Name   *string            `json:"name"`
	Fields *[]sport.StatField `json:"fields"`
	Active *bool              `json:"active"`
}

// actor is the calling admin plus the audit writer bound to the request. Every admin write
// calls log (admin_audit_log) and audit.Config (config_audit) inside its transaction.
type actor struct {
	id  int64
	log func(tx *sql.Tx, action, targetType, targetID string, details any) error
}

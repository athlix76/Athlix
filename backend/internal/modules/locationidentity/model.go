package locationidentity

import (
	"database/sql"
	"time"

	"athlix/backend/internal/platform/userdir"
)

// Identity types, statuses and proof holders (ENUMs of user_location_identities).
const (
	typeNative    = "native"
	typeResidence = "residence"

	statusPending    = "pending"
	statusVerified   = "verified"
	statusRejected   = "rejected"
	statusExpired    = "expired"
	statusSuperseded = "superseded"
	statusRevoked    = "revoked"
	statusWithdrawn  = "withdrawn"

	holderSelf     = "self"
	holderGuardian = "guardian"

	proofDigiLocker    = "digilocker_aadhaar"
	proofMaskedAadhaar = "masked_aadhaar"

	sourceDigiLocker = "digilocker"
	sourceUpload     = "upload"

	rejectUnmaskedAadhaar = "unmasked_aadhaar"

	noticeDocType = "location_identity_notice"
)

var allStatuses = []string{statusPending, statusVerified, statusRejected, statusExpired, statusSuperseded, statusRevoked, statusWithdrawn}

// Fallback defaults: each equals the seed value in migrations/0008_location_identity.sql.
const (
	defEnabled            = true
	defValidityMonths     = 12
	defCooldownDays       = 180
	defNativeChange       = true
	defRetentionDays      = 30
	defReminderDays       = 30
	defGuardianAllowed    = true
	defUtilityBillMaxDays = 90
)

var (
	defNativeProofs     = []string{"masked_aadhaar", "birth_certificate"}
	defResidenceProofs  = []string{"rent_agreement", "utility_bill", "bank_statement", "voter_id", "employer_letter", "institution_id"}
	defRejectionReasons = []string{"unclear_document", "name_mismatch", "address_mismatch", "district_mismatch", "document_too_old", "unmasked_aadhaar", "wrong_document_type", "other"}
)

// Technical limits (not business numbers).
const (
	jobBatch        = 500 // docs/architecture.md#scheduled-jobs: <= 500 rows per run
	userRowsLimit   = 100 // rows loaded for GET /me/location-identities
	historyLimit    = 20  // earlier rows shown in AdminLocationDetail.history
	adminPageSize   = 50
	maxNoteLen      = 255 // rejection_reason / review_note columns
	maxDistrictName = 80  // districts.name column
	maxAliases      = 20
)

// State is the public State shape.
type State struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
	IsUT bool   `json:"isUT"`
}

// AdminState = State + admin fields (GET /admin/geo/states, inactive states included).
type AdminState struct {
	State
	LgdCode *int64 `json:"lgdCode"`
	Active  bool   `json:"active"`
}

// District is the public District shape.
type District struct {
	ID      int64  `json:"id"`
	StateID int64  `json:"stateId"`
	Name    string `json:"name"`
}

// AdminDistrict = District + admin fields.
type AdminDistrict struct {
	District
	LgdCode *int64   `json:"lgdCode"`
	Aliases []string `json:"aliases"`
	Active  bool     `json:"active"`
}

// LocationIdentity is the user-facing shape of one row.
type LocationIdentity struct {
	ID              int64      `json:"id"`
	IdentityType    string     `json:"identityType"`
	District        District   `json:"district"`
	State           State      `json:"state"`
	Source          string     `json:"source"`
	ProofType       string     `json:"proofType"`
	ProofHolder     string     `json:"proofHolder"`
	AadhaarLast4    *string    `json:"aadhaarLast4"`
	Status          string     `json:"status"`
	RejectionCode   *string    `json:"rejectionCode"`
	RejectionReason *string    `json:"rejectionReason"`
	SubmittedAt     time.Time  `json:"submittedAt"`
	ReviewedAt      *time.Time `json:"reviewedAt"`
	VerifiedAt      *time.Time `json:"verifiedAt"`
	ExpiresAt       *time.Time `json:"expiresAt"`
}

// IdentitySlot is one type's view in LocationIdentities.
type IdentitySlot struct {
	DisplayStatus string            `json:"displayStatus"`
	Current       *LocationIdentity `json:"current"`
	Pending       *LocationIdentity `json:"pending"`
	CanSubmit     bool              `json:"canSubmit"`
	NextChangeAt  *time.Time        `json:"nextChangeAt"`
	ExpiresInDays *int              `json:"expiresInDays"`
}

// Notice is the active location_identity_notice policy document.
type Notice struct {
	PolicyDocumentID int64  `json:"policyDocumentId"`
	Version          int64  `json:"version"`
	Title            string `json:"title"`
}

// ProofTypes lists accepted upload proofs per type.
type ProofTypes struct {
	Native    []string `json:"native"`
	Residence []string `json:"residence"`
}

// LocationIdentities is GET /me/location-identities.
type LocationIdentities struct {
	Native               IdentitySlot `json:"native"`
	Residence            IdentitySlot `json:"residence"`
	Notice               *Notice      `json:"notice"`
	ProofTypes           ProofTypes   `json:"proofTypes"`
	GuardianProofAllowed bool         `json:"guardianProofAllowed"`
}

// LocationIdentityInput is the POST /me/location-identities body.
type LocationIdentityInput struct {
	IdentityType           string `json:"identityType"`
	DistrictID             int64  `json:"districtId"`
	ProofType              string `json:"proofType"`
	ProofUploadID          int64  `json:"proofUploadId"`
	ProofHolder            string `json:"proofHolder"`
	NoticePolicyDocumentID int64  `json:"noticePolicyDocumentId"`
}

// AdminRef is the reviewer shown on a detail.
type AdminRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// AdminLocationRow is one queue row.
type AdminLocationRow struct {
	ID           int64           `json:"id"`
	User         userdir.Summary `json:"user"`
	IsMinor      bool            `json:"isMinor"`
	IdentityType string          `json:"identityType"`
	District     District        `json:"district"`
	State        State           `json:"state"`
	ProofType    string          `json:"proofType"`
	ProofHolder  string          `json:"proofHolder"`
	Status       string          `json:"status"`
	SubmittedAt  time.Time       `json:"submittedAt"`
}

// Checklist helps the reviewer.
type Checklist struct {
	UtilityBillMaxAgeDays int      `json:"utilityBillMaxAgeDays"`
	RejectionReasons      []string `json:"rejectionReasons"`
}

// AdminLocationDetail = AdminLocationRow + review fields.
type AdminLocationDetail struct {
	AdminLocationRow
	ProofURL         *string            `json:"proofUrl"`
	ProofContentType *string            `json:"proofContentType"`
	ProofPurgedAt    *time.Time         `json:"proofPurgedAt"`
	AadhaarLast4     *string            `json:"aadhaarLast4"`
	RejectionCode    *string            `json:"rejectionCode"`
	RejectionReason  *string            `json:"rejectionReason"`
	ReviewNote       *string            `json:"reviewNote"`
	ReviewedAt       *time.Time         `json:"reviewedAt"`
	ReviewedBy       *AdminRef          `json:"reviewedBy"`
	VerifiedAt       *time.Time         `json:"verifiedAt"`
	ExpiresAt        *time.Time         `json:"expiresAt"`
	ProfileDistrict  *string            `json:"profileDistrict"`
	History          []LocationIdentity `json:"history"`
	Checklist        Checklist          `json:"checklist"`
}

// Admin request bodies.
type approveBody struct {
	AadhaarLast4 *string `json:"aadhaarLast4"`
	Note         *string `json:"note"`
}

type rejectBody struct {
	RejectionCode   string  `json:"rejectionCode"`
	RejectionReason *string `json:"rejectionReason"`
}

type revokeBody struct {
	Reason string `json:"reason"`
}

type districtCreateBody struct {
	StateID int64    `json:"stateId"`
	Name    string   `json:"name"`
	LgdCode *int64   `json:"lgdCode"`
	Aliases []string `json:"aliases"`
}

type districtUpdateBody struct {
	Name    *string   `json:"name"`
	LgdCode *int64    `json:"lgdCode"`
	Aliases *[]string `json:"aliases"`
	Active  *bool     `json:"active"`
}

// DistrictFilter is GET /admin/geo/districts.
type DistrictFilter struct {
	StateID         int64
	Q               string
	IncludeInactive bool
}

// QueueFilter is GET /admin/location-identities.
type QueueFilter struct {
	Status       string
	IdentityType string
	StateID      int64
	DistrictID   int64
}

// identityRow is one user_location_identities row joined with its district and state.
type identityRow struct {
	ID              int64
	UserID          int64
	Type            string
	DistrictID      int64
	DistrictName    string
	StateID         int64
	StateCode       string
	StateName       string
	StateIsUT       bool
	ProofType       string
	ProofHolder     string
	ProofUploadID   sql.NullInt64
	AadhaarLast4    sql.NullString
	NoticeVersion   sql.NullInt64
	Status          string
	RejectionCode   sql.NullString
	RejectionReason sql.NullString
	SubmittedAt     time.Time
	ReviewedAt      sql.NullTime
	ReviewedBy      sql.NullInt64
	VerifiedAt      sql.NullTime
	ExpiresAt       sql.NullTime
	ProofPurgedAt   sql.NullTime
	UpdatedAt       time.Time
}

// adminRow adds the admin-only joined columns.
type adminRow struct {
	identityRow
	IsMinor          bool
	ReviewNote       sql.NullString
	ProfileDistrict  sql.NullString
	ReviewerName     sql.NullString
	ProofContentType sql.NullString
}

// districtRow is a districts row joined with its state.
type districtRow struct {
	ID        int64
	StateID   int64
	Name      string
	LgdCode   sql.NullInt64
	Aliases   []string
	Active    bool
	StateCode string
	StateName string
	StateIsUT bool
}

func ptrStr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	s := n.String
	return &s
}

func ptrTime(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time.UTC()
	return &t
}

func (r identityRow) district() District {
	return District{ID: r.DistrictID, StateID: r.StateID, Name: r.DistrictName}
}

func (r identityRow) state() State {
	return State{ID: r.StateID, Code: r.StateCode, Name: r.StateName, IsUT: r.StateIsUT}
}

func (r identityRow) toAPI() LocationIdentity {
	src := sourceUpload
	if r.ProofType == proofDigiLocker {
		src = sourceDigiLocker
	}
	return LocationIdentity{
		ID:              r.ID,
		IdentityType:    r.Type,
		District:        r.district(),
		State:           r.state(),
		Source:          src,
		ProofType:       r.ProofType,
		ProofHolder:     r.ProofHolder,
		AadhaarLast4:    ptrStr(r.AadhaarLast4),
		Status:          r.Status,
		RejectionCode:   ptrStr(r.RejectionCode),
		RejectionReason: ptrStr(r.RejectionReason),
		SubmittedAt:     r.SubmittedAt.UTC(),
		ReviewedAt:      ptrTime(r.ReviewedAt),
		VerifiedAt:      ptrTime(r.VerifiedAt),
		ExpiresAt:       ptrTime(r.ExpiresAt),
	}
}

func (r districtRow) toAdmin() AdminDistrict {
	out := AdminDistrict{District: District{ID: r.ID, StateID: r.StateID, Name: r.Name}, Aliases: r.Aliases, Active: r.Active}
	if out.Aliases == nil {
		out.Aliases = []string{}
	}
	if r.LgdCode.Valid {
		v := r.LgdCode.Int64
		out.LgdCode = &v
	}
	return out
}

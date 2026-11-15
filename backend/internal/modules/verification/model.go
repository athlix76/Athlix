package verification

import "time"

// Providers (app_config verification.provider and identity_verifications.provider).
const (
	ProviderDirect     = "digilocker_direct"
	ProviderAggregator = "aggregator"
	ProviderMock       = "mock"
)

// Failure reasons (VerificationStatus.failureReason and the deep link reason).
const (
	ReasonTooYoung      = "too_young"
	ReasonAlreadyLinked = "already_linked"
	ReasonProviderError = "provider_error"
	ReasonCancelled     = "cancelled"
	ReasonNameMismatch  = "name_mismatch"
)

// Deep link result statuses.
const (
	ResultVerified = "verified"
	ResultMinor    = "minor"
	ResultFailed   = "failed"
)

// MockOutcomes is the closed list for POST /verification/mock/complete.
var MockOutcomes = []string{"adult", "minor", "too_young", "duplicate", "failed"}

// Status is the VerificationStatus JSON in docs/api.md.
type Status struct {
	Status            string     `json:"status"`
	Provider          *string    `json:"provider"`
	AadhaarLast4      *string    `json:"aadhaarLast4"`
	VerifiedAt        *time.Time `json:"verifiedAt"`
	FailureReason     *string    `json:"failureReason"`
	IsMinor           bool       `json:"isMinor"`
	AttemptsLeftToday int        `json:"attemptsLeftToday"`
}

// StartResult is the response of POST /verification/digilocker/start.
type StartResult struct {
	AuthorizationURL string    `json:"authorizationUrl"`
	ExpiresAt        time.Time `json:"expiresAt"`
}

// Identity is what a provider returns. It lives in memory only for the length of one
// callback: the service keeps last4, encrypted name + DOB, gender and the subject hash, maps
// the address to a district id, and drops everything else. Never log it.
type Identity struct {
	SubjectID    string
	AadhaarLast4 string
	Name         string
	DOB          time.Time
	Gender       string // M | F | T | U
	StateName    string // address: used only for district matching, then discarded
	DistrictName string
}

// outcome is the result of one attempt.
type outcome struct {
	Result           string // verified | minor | failed
	Reason           string // failure reason when Result is failed
	IsMinor          bool
	NativeDistrictID int64
}

type mockBody struct {
	Outcome          string `json:"outcome"`
	NativeDistrictID *int64 `json:"nativeDistrictId"`
}

// pendingState is stored in Redis under dl:state:<sha256(state)> until the callback.
type pendingState struct {
	UserID    int64  `json:"userId"`
	AttemptID int64  `json:"attemptId"`
	Provider  string `json:"provider"`
	Verifier  string `json:"verifier,omitempty"`
	Ref       string `json:"ref,omitempty"`
}

package guardian

// Relations is the closed list for guardian_consents.relation.
var Relations = []string{"mother", "father", "legal_guardian", "other"}

// Consent statuses (guardian_consents.status).
const (
	StatusPending = "pending"
	StatusGranted = "granted"
	StatusRevoked = "revoked"
	StatusExpired = "expired"
)

// requestBody is POST /onboarding/guardian/request.
type requestBody struct {
	GuardianName     string `json:"guardianName"`
	GuardianMobile   string `json:"guardianMobile"`
	Relation         string `json:"relation"`
	PolicyDocumentID int64  `json:"policyDocumentId"`
}

// verifyBody is POST /onboarding/guardian/verify.
type verifyBody struct {
	ConsentID int64  `json:"consentId"`
	OTP       string `json:"otp"`
}

// Ticket is the response of POST /onboarding/guardian/request.
type Ticket struct {
	ConsentID            int64  `json:"consentId"`
	GuardianMobileMasked string `json:"guardianMobileMasked"`
	ExpiresInSeconds     int    `json:"expiresInSeconds"`
	ResendAfterSeconds   int    `json:"resendAfterSeconds"`
}

// withdrawVerifyBody is POST /me/guardian/withdraw/verify.
type withdrawVerifyBody struct {
	ConsentID int64  `json:"consentId"`
	OTP       string `json:"otp"`
}

// adminRevokeBody is POST /admin/users/:id/guardian-consent/revoke.
type adminRevokeBody struct {
	Reason string `json:"reason"`
}

// Revoked is the response of the admin revoke.
type Revoked struct {
	UserID                int64  `json:"userId"`
	ConsentID             int64  `json:"consentId"`
	GuardianConsentStatus string `json:"guardianConsentStatus"`
}

// Reasons stored in guardian_consents.revoked_reason by the self-service withdrawal.
const reasonGuardianWithdrew = "guardian_withdrew"

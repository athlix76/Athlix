package user

import "time"

// Roles a user can hold.
var validRoles = []string{"player", "coach", "organiser"}

// PolicyRef is a policy version the user still has to accept.
type PolicyRef struct {
	ID      int64  `json:"id"`
	DocType string `json:"docType"`
	Version int    `json:"version"`
	Title   string `json:"title"`
}

// Me is the signed-in user (docs/api.md "Me").
type Me struct {
	ID                    int64       `json:"id"`
	DisplayName           *string     `json:"displayName"`
	MobileMasked          string      `json:"mobileMasked"`
	PrimaryRole           *string     `json:"primaryRole"`
	Roles                 []string    `json:"roles"`
	Status                string      `json:"status"`
	OnboardingStep        string      `json:"onboardingStep"`
	VerificationStatus    string      `json:"verificationStatus"`
	IsMinor               bool        `json:"isMinor"`
	GuardianConsentStatus string      `json:"guardianConsentStatus"`
	PhotoURL              *string     `json:"photoUrl"`
	StoreAccountToken     string      `json:"storeAccountToken"`
	PendingPolicies       []PolicyRef `json:"pendingPolicies"`
	CreatedAt             time.Time   `json:"createdAt"`
}

// NotificationPrefs are the push categories a user receives.
type NotificationPrefs struct {
	Messages  bool `json:"messages"`
	Events    bool `json:"events"`
	Social    bool `json:"social"`
	Marketing bool `json:"marketing"`
}

// Privacy holds who may message the user and whether they appear in search.
type Privacy struct {
	AllowDmsFrom string `json:"allowDmsFrom"` // everyone | followers | mutual | nobody
	ShowInSearch bool   `json:"showInSearch"`
}

// Settings is GET/PUT /me/settings.
type Settings struct {
	NotificationPrefs NotificationPrefs `json:"notificationPrefs"`
	Privacy           Privacy           `json:"privacy"`
}

// DefaultSettings applies when the user never saved settings (or a key is missing).
func DefaultSettings() Settings {
	return Settings{
		NotificationPrefs: NotificationPrefs{Messages: true, Events: true, Social: true, Marketing: false},
		Privacy:           Privacy{AllowDmsFrom: "everyone", ShowInSearch: true},
	}
}

// AllowDmsFrom values.
var allowDmsValues = []string{"everyone", "followers", "mutual", "nobody"}

type settingsUpdate struct {
	NotificationPrefs *struct {
		Messages  *bool `json:"messages"`
		Events    *bool `json:"events"`
		Social    *bool `json:"social"`
		Marketing *bool `json:"marketing"`
	} `json:"notificationPrefs"`
	Privacy *struct {
		AllowDmsFrom *string `json:"allowDmsFrom"`
		ShowInSearch *bool   `json:"showInSearch"`
	} `json:"privacy"`
}

type acceptPoliciesRequest struct {
	PolicyDocumentIDs []int64 `json:"policyDocumentIds"`
}

type addRoleRequest struct {
	Role string `json:"role"`
}

type onboardingRoleRequest struct {
	Role            string   `json:"role"`
	AdditionalRoles []string `json:"additionalRoles"`
}

// AdminUserRow is one row of GET /admin/users.
type AdminUserRow struct {
	ID                    int64      `json:"id"`
	DisplayName           *string    `json:"displayName"`
	MobileMasked          string     `json:"mobileMasked"`
	PrimaryRole           *string    `json:"primaryRole"`
	Roles                 []string   `json:"roles"`
	Status                string     `json:"status"`
	VerificationStatus    string     `json:"verificationStatus"`
	IsMinor               bool       `json:"isMinor"`
	GuardianConsentStatus string     `json:"guardianConsentStatus"`
	OnboardingStep        string     `json:"onboardingStep"`
	CreatedAt             time.Time  `json:"createdAt"`
	LastLoginAt           *time.Time `json:"lastLoginAt"`
	suspendedReason       *string
}

// AdminVerification is the verification block of AdminUserDetail.
type AdminVerification struct {
	Status       string     `json:"status"`
	Provider     string     `json:"provider"`
	AadhaarLast4 *string    `json:"aadhaarLast4"`
	VerifiedName *string    `json:"verifiedName"`
	Gender       *string    `json:"gender"`
	VerifiedAt   *time.Time `json:"verifiedAt"`
}

// AdminGuardian is the guardian block of AdminUserDetail.
type AdminGuardian struct {
	Relation     string     `json:"relation"`
	MobileMasked string     `json:"mobileMasked"`
	Status       string     `json:"status"`
	GrantedAt    *time.Time `json:"grantedAt"`
}

// AdminCounts is the activity block of AdminUserDetail.
type AdminCounts struct {
	EventsOrganised int `json:"eventsOrganised"`
	Registrations   int `json:"registrations"`
	Posts           int `json:"posts"`
	ReportsAgainst  int `json:"reportsAgainst"`
}

// AdminUserDetail is GET /admin/users/:id.
type AdminUserDetail struct {
	AdminUserRow
	Verification    *AdminVerification `json:"verification"`
	Guardian        *AdminGuardian     `json:"guardian"`
	Counts          AdminCounts        `json:"counts"`
	SuspendedReason *string            `json:"suspendedReason"`
}

type suspendRequest struct {
	Reason string `json:"reason"`
}

// adminUserFilter is the parsed query of GET /admin/users.
type adminUserFilter struct {
	ID                 int64
	MobileHash         []byte
	NamePrefix         string
	Role               string
	Status             string
	VerificationStatus string
	IsMinor            *bool
	BeforeID           int64
	Limit              int
}

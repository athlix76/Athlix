package admin

import (
	"encoding/json"
	"time"

	"athlix/backend/internal/modules/auth/tokens"
)

// Permissions is the closed list of admin permission strings (docs/security.md). "*" is reserved for the seeded super_admin role.
var Permissions = []string{
	"dashboard.read", "admins.manage", "audit.read", "config.read", "config.write",
	"policies.write", "sports.write", "fees.write", "plans.write", "users.read", "users.write",
	"organisers.review", "events.review", "payments.read", "payments.refund", "reports.read",
	"reports.resolve", "moderation.read_messages", "communities.moderate", "ads.write",
	"notifications.broadcast", "location.review", "geo.write",
}

// SuperAdminRole is the seeded role whose permissions are ["*"]; it cannot be edited.
const SuperAdminRole = "super_admin"

// RoleRef is the {id, name} pair inside AdminUser.
type RoleRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// AdminUser is AdminMe / AdminUser in docs/api.md.
type AdminUser struct {
	ID          int64      `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Role        RoleRef    `json:"role"`
	Permissions []string   `json:"permissions"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
}

// Role is an admin_roles row.
type Role struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	Permissions []string `json:"permissions"`
}

// Credentials are shown once, when an admin is created or reset. Never log or store them.
type Credentials struct {
	TemporaryPassword string `json:"temporaryPassword"`
	TotpURI           string `json:"totpUri"`
}

// CreatedAdmin is the POST /admin/admins response.
type CreatedAdmin struct {
	Admin AdminUser `json:"admin"`
	Credentials
}

// LoginResult is the POST /admin/auth/login response.
type LoginResult struct {
	Tokens tokens.Pair `json:"tokens"`
	Admin  AdminUser   `json:"admin"`
}

// AuditEntry is one row of GET /admin/audit-log.
type AuditEntry struct {
	ID         int64           `json:"id"`
	Admin      AuditAdmin      `json:"admin"`
	Action     string          `json:"action"`
	TargetType *string         `json:"targetType"`
	TargetID   *string         `json:"targetId"`
	Details    json.RawMessage `json:"details"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// AuditAdmin is the {id, name} of the acting admin.
type AuditAdmin struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Dashboard is GET /admin/dashboard.
type Dashboard struct {
	Users struct {
		Total        int64 `json:"total"`
		Verified     int64 `json:"verified"`
		Minors       int64 `json:"minors"`
		NewLast7Days int64 `json:"newLast7Days"`
	} `json:"users"`
	Events struct {
		Published     int64 `json:"published"`
		PendingReview int64 `json:"pendingReview"`
		Upcoming      int64 `json:"upcoming"`
	} `json:"events"`
	Organisers struct {
		PendingReview int64 `json:"pendingReview"`
	} `json:"organisers"`
	Reports struct {
		Open int64 `json:"open"`
	} `json:"reports"`
	Revenue struct {
		ListingPaidPaiseLast30Days     int64 `json:"listingPaidPaiseLast30Days"`
		ListingRefundedPaiseLast30Days int64 `json:"listingRefundedPaiseLast30Days"`
		ActiveSubscriptions            int64 `json:"activeSubscriptions"`
	} `json:"revenue"`
	Communities struct {
		Active int64 `json:"active"`
	} `json:"communities"`
}

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TotpCode string `json:"totpCode"`
}

type refreshBody struct {
	RefreshToken string `json:"refreshToken"`
}

type createAdminBody struct {
	Email  string `json:"email"`
	Name   string `json:"name"`
	RoleID int64  `json:"roleId"`
}

type updateAdminBody struct {
	Name   *string `json:"name"`
	RoleID *int64  `json:"roleId"`
	Status *string `json:"status"`
}

type createRoleBody struct {
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	Permissions []string `json:"permissions"`
}

type updateRoleBody struct {
	Description *string   `json:"description"`
	Permissions *[]string `json:"permissions"`
}

type auditFilter struct {
	AdminID    int64
	Action     string
	TargetType string
	TargetID   string
	BeforeID   int64
	Limit      int
}

// adminAuth is the login row (secrets stay inside the service).
type adminAuth struct {
	ID           int64
	Email        string // set by authByID only
	PasswordHash string
	TotpEnc      []byte
	RoleID       int64
	Status       string
	FailedLogins int
	LockedUntil  *time.Time
}

// changePasswordBody is POST /admin/me/password. Never log it.
type changePasswordBody struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
	TotpCode        string `json:"totpCode"`
}

// ChangePasswordResult is the POST /admin/me/password response: a fresh session, because
// every other session of the admin is revoked.
type ChangePasswordResult struct {
	Tokens tokens.Pair `json:"tokens"`
}

// ClientInfo is the caller's network details recorded with a login attempt.
type ClientInfo struct {
	IP        string
	UserAgent string
}

// Login outcomes and reason codes (admin_login_events). Reasons are for audit readers
// only; the login response never says which check failed.
const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"

	ReasonOK           = "ok"
	ReasonUnknownEmail = "unknown_email"
	ReasonDisabled     = "disabled"
	ReasonLocked       = "locked"
	ReasonBadPassword  = "bad_password"
	ReasonBadTOTP      = "bad_totp"
	ReasonTOTPReplayed = "totp_replayed"
	ReasonNoTOTP       = "totp_not_set"
)

// LoginEvent is one row of GET /admin/login-events.
type LoginEvent struct {
	ID        int64       `json:"id"`
	Admin     *AuditAdmin `json:"admin"`
	Outcome   string      `json:"outcome"`
	Reason    string      `json:"reason"`
	IP        *string     `json:"ip"`
	UserAgent *string     `json:"userAgent"`
	CreatedAt time.Time   `json:"createdAt"`
}

type loginEventFilter struct {
	AdminID   int64
	EmailHash []byte
	Outcome   string
	BeforeID  int64
	Limit     int
}

type loginEventRow struct {
	AdminID   int64 // 0 = unknown email
	EmailHash []byte
	IP        string
	UserAgent string
	Outcome   string
	Reason    string
}

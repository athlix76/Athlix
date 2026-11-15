package community

import (
	"encoding/json"
	"time"

	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/userdir"
)

// Visibility, join policy, member status and role values (conversations / conversation_members enums).
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"

	JoinOpen     = "open"
	JoinApproval = "approval"
	JoinInvite   = "invite"

	StatusActive   = "active"
	StatusArchived = "archived"
	StatusRemoved  = "removed"

	MemberActive  = "active"
	MemberPending = "pending"
	MemberInvited = "invited"
	MemberLeft    = "left"
	MemberRemoved = "removed"
	MemberBanned  = "banned"

	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"

	// MyStatusNone is shown when the caller has no membership row (or left / was removed).
	MyStatusNone = "none"
)

// Event types this module publishes besides the events package constants. They are listed in
// docs/architecture.md#domain-events; the payload JSON is a superset of
// events.CommunityJoinPayload so existing consumers can still decode it.
const (
	EventInvited              = "community.invited"
	EventOwnershipTransferred = "community.ownership_transferred"
)

// InvitedPayload is the community.invited payload: one event per newly invited user.
type InvitedPayload struct {
	events.CommunityJoinPayload       // conversationId, userId = the invited user, ownerId
	InviterID                   int64 `json:"inviterId"`
}

// OwnershipPayload is the community.ownership_transferred payload. userId is the new owner,
// ownerId the new owner too (so CommunityJoinPayload readers see the current owner), and
// previousOwnerId the old owner, who is now an admin.
type OwnershipPayload struct {
	events.CommunityJoinPayload
	PreviousOwnerID int64 `json:"previousOwnerId"`
}

// Field limits (column sizes in migrations/0004_social_chat.sql).
const (
	titleMin       = 3
	titleMax       = 80
	descriptionMax = 500
	placeMax       = 64
	maxInviteBatch = 50
	reasonMax      = 500
	defaultPage    = 20
)

// SportRef is the api.md SportRef shape.
type SportRef struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// Community is the api.md Community shape.
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
	CreatedAt   time.Time        `json:"createdAt"`
}

// AdminCommunity is Community + {status, removalReason} for GET /admin/communities.
type AdminCommunity struct {
	Community
	Status        string  `json:"status"`
	RemovalReason *string `json:"removalReason"` // set by POST /admin/communities/:id/remove
}

// JoinResult is the POST /communities/:id/join response.
type JoinResult struct {
	MyStatus string `json:"myStatus"`
}

// JoinRequest is one row of GET /communities/:id/join-requests.
type JoinRequest struct {
	User        userdir.Summary `json:"user"`
	RequestedAt time.Time       `json:"requestedAt"`
}

// InviteResult is the POST /communities/:id/invites response.
type InviteResult struct {
	Invited []int64 `json:"invited"`
	Skipped []int64 `json:"skipped"`
}

// Opt is an optional JSON field: Set reports the key was present, Null that it was null.
// PUT uses it so a missing field means "unchanged" and null means "clear".
type Opt[T any] struct {
	Set  bool
	Null bool
	V    T
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Null = true
		return nil
	}
	return json.Unmarshal(b, &o.V)
}

// Input is CommunityInput (POST: create, PUT: partial update).
type Input struct {
	Title          Opt[string] `json:"title"`
	Description    Opt[string] `json:"description"`
	AvatarUploadID Opt[int64]  `json:"avatarUploadId"`
	SportID        Opt[int64]  `json:"sportId"`
	City           Opt[string] `json:"city"`
	State          Opt[string] `json:"state"`
	Visibility     Opt[string] `json:"visibility"`
	JoinPolicy     Opt[string] `json:"joinPolicy"`
}

// InviteInput is the POST /communities/:id/invites body.
type InviteInput struct {
	UserIDs []int64 `json:"userIds"`
}

// RoleInput is the PUT /communities/:id/members/:userId/role body.
type RoleInput struct {
	Role string `json:"role"`
}

// TransferInput is the POST /communities/:id/transfer-ownership body.
type TransferInput struct {
	UserID int64 `json:"userId"`
}

// RemoveInput is the POST /admin/communities/:id/remove body.
type RemoveInput struct {
	Reason string `json:"reason"`
}

// ListFilter holds the GET /communities query filters.
type ListFilter struct {
	Q       string
	SportID int64
	State   string
}

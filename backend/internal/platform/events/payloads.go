package events

// Payload shapes. Publishers MUST use these structs; handlers decode with Event.Decode.
// Ids and flags only: no names, phone numbers, message text or tokens.

// UserPayload is used by user.registered, guardian.consent_granted.
type UserPayload struct {
	UserID int64 `json:"userId"`
}

// VerificationPayload is used by verification.completed.
type VerificationPayload struct {
	UserID  int64  `json:"userId"`
	Status  string `json:"status"` // verified | failed
	IsMinor bool   `json:"isMinor"`
	// NativeDistrictID is the districts.id matched from the DigiLocker Aadhaar address
	// (0 when it could not be matched). The address itself is never stored or sent.
	NativeDistrictID int64 `json:"nativeDistrictId,omitempty"`
}

// FollowPayload is used by follow.created.
type FollowPayload struct {
	FollowerID int64 `json:"followerId"`
	FolloweeID int64 `json:"followeeId"`
}

// EventPayload is used by event.submitted/published/rejected/cancelled/updated.
type EventPayload struct {
	EventID     int64  `json:"eventId"`
	OrganiserID int64  `json:"organiserId"`
	SportID     int64  `json:"sportId"`
	Reason      string `json:"reason,omitempty"` // rejection / cancel reason (organiser-visible text)
}

// RegistrationPayload is used by registration.created/withdrawn/payment_marked.
type RegistrationPayload struct {
	RegistrationID  int64  `json:"registrationId"`
	EventID         int64  `json:"eventId"`
	UserID          int64  `json:"userId"`
	OrganiserID     int64  `json:"organiserId"`
	Status          string `json:"status"`                    // registered | waitlisted | withdrawn | rejected
	PaymentStatus   string `json:"paymentStatus"`             // unpaid | paid_online | paid_offline | waived
	EligibilityFlag string `json:"eligibilityFlag,omitempty"` // registration.eligibility_flagged only
}

// ListingPayload is used by listing.paid/refunded.
type ListingPayload struct {
	OrderID     int64 `json:"orderId"`
	EventID     int64 `json:"eventId"`
	OrganiserID int64 `json:"organiserId"`
	AmountPaise int64 `json:"amountPaise"`
}

// OrganiserPayload is used by organiser.approved/rejected.
type OrganiserPayload struct {
	UserID int64  `json:"userId"`
	Reason string `json:"reason,omitempty"`
}

// MessagePayload is used by message.created.
type MessagePayload struct {
	MessageID      int64 `json:"messageId"`
	ConversationID int64 `json:"conversationId"`
	SenderID       int64 `json:"senderId"`
}

// PostPayload is used by post.liked/commented.
type PostPayload struct {
	PostID    int64 `json:"postId"`
	AuthorID  int64 `json:"authorId"`
	ActorID   int64 `json:"actorId"`
	CommentID int64 `json:"commentId,omitempty"`
}

// ConsultationPayload is used by consultation.requested/responded.
type ConsultationPayload struct {
	RequestID int64  `json:"requestId"`
	CoachID   int64  `json:"coachId"`
	PlayerID  int64  `json:"playerId"`
	Status    string `json:"status"`
	// ResponderID is who changed the request (consultation.responded only): the coach for
	// accept/decline/complete, the player for cancel. Consumers notify the other party.
	ResponderID int64 `json:"responderId,omitempty"`
}

// ReportPayload is used by report.created.
type ReportPayload struct {
	ReportID   int64  `json:"reportId"`
	TargetType string `json:"targetType"`
	TargetID   int64  `json:"targetId"`
}

// SubscriptionPayload is used by subscription.activated/expired.
type SubscriptionPayload struct {
	SubscriptionID int64 `json:"subscriptionId"`
	UserID         int64 `json:"userId"`
	PlanID         int64 `json:"planId"`
}

// CommunityJoinPayload is used by community.join_requested/join_approved.
type CommunityJoinPayload struct {
	ConversationID int64 `json:"conversationId"`
	UserID         int64 `json:"userId"`
	OwnerID        int64 `json:"ownerId"`
}

// LocationIdentityPayload is used by location_identity.changed and location_identity.expiring.
type LocationIdentityPayload struct {
	IdentityID    int64  `json:"identityId"`
	UserID        int64  `json:"userId"`
	IdentityType  string `json:"identityType"` // native | residence
	DistrictID    int64  `json:"districtId"`
	Status        string `json:"status"`                  // new status (pending | verified | rejected | expired | superseded | revoked | withdrawn)
	PrevStatus    string `json:"prevStatus,omitempty"`    // status before the change
	RejectionCode string `json:"rejectionCode,omitempty"` // rejected only
	ExpiresAt     string `json:"expiresAt,omitempty"`     // RFC 3339, residence only
}

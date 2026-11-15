package notification

import (
	"context"
	"fmt"
	"strconv"

	"athlix/backend/internal/platform/events"
)

// handlers maps every consumed event type to its handler (architecture.md "Domain events").
func (s *Service) handlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.VerificationCompleted:   s.onVerification,
		events.GuardianConsentGranted:  s.onGuardianConsent,
		events.GuardianConsentRevoked:  s.onGuardianConsent,
		events.FollowCreated:           s.onFollow,
		events.OrganiserApproved:       s.onOrganiser,
		events.OrganiserRejected:       s.onOrganiser,
		events.EventSubmitted:          s.onEventForOrganiser,
		events.EventPublished:          s.onEventForOrganiser,
		events.EventRejected:           s.onEventForOrganiser,
		events.EventCancelled:          s.onEventCancelled,
		events.EventUpdated:            s.onEventUpdated,
		events.RegistrationCreated:     s.onRegistrationForOrganiser,
		events.RegistrationWithdrawn:   s.onRegistrationForOrganiser,
		events.RegistrationPromoted:    s.onRegistrationForPlayer,
		events.RegistrationPaymentMark: s.onRegistrationForPlayer,
		events.ListingPaid:             s.onListing,
		events.ListingRefunded:         s.onListing,
		events.MessageCreated:          s.onMessage,
		events.PostLiked:               s.onPost,
		events.PostCommented:           s.onPost,
		events.ConsultationRequested:   s.onConsultation,
		events.ConsultationResponded:   s.onConsultation,
		events.SubscriptionActivated:   s.onSubscription,
		events.SubscriptionExpired:     s.onSubscription,
		events.CommunityJoinRequested:  s.onCommunityJoin,
		events.CommunityJoinApproved:   s.onCommunityJoin,
		EventBroadcastPush:             s.onBroadcastPush,

		events.LocationIdentityChanged:        s.onLocationIdentityChanged,
		events.LocationIdentityExpiring:       s.onLocationIdentityExpiring,
		events.RegistrationEligibilityFlagged: s.onEligibilityFlagged,

		events.UserDeleted: s.onUserDeleted,
	}
}

// decode returns false for a malformed payload: retrying would never succeed, so it is dropped.
func decode(e events.Event, out any) bool { return e.Decode(out) == nil }

// displayName returns the user's display name ("Someone" when missing or deleted).
func (s *Service) displayName(ctx context.Context, id int64) (string, error) {
	sum, ok, err := s.d.Users.Summary(ctx, id)
	if err != nil {
		return "", err
	}
	if !ok || sum.DisplayName == "" {
		return "Someone", nil
	}
	return clip(sum.DisplayName, 60), nil
}

// ---- account ----

func (s *Service) onVerification(ctx context.Context, e events.Event) error {
	var p events.VerificationPayload
	if !decode(e, &p) {
		return nil
	}
	n := note{Type: e.Type, Title: "You're verified", Body: "Your identity is verified. You can now use all of Athlix.", Category: categoryAccount}
	if p.Status != "verified" {
		n.Title, n.Body = "Verification didn't complete", "Please try verifying your identity again."
	}
	return s.notify(ctx, e, []int64{p.UserID}, n)
}

func (s *Service) onGuardianConsent(ctx context.Context, e events.Event) error {
	var p events.UserPayload
	if !decode(e, &p) {
		return nil
	}
	n := note{Type: e.Type, Title: "Guardian consent received", Body: "Your guardian approved your account.", Category: categoryAccount}
	if e.Type == events.GuardianConsentRevoked {
		n.Title, n.Body = "Guardian consent withdrawn", "Ask your guardian to approve your account again to keep using Athlix."
	}
	return s.notify(ctx, e, []int64{p.UserID}, n)
}

func (s *Service) onSubscription(ctx context.Context, e events.Event) error {
	var p events.SubscriptionPayload
	if !decode(e, &p) {
		return nil
	}
	n := note{Type: e.Type, Title: "Your plan is active", Body: "Your premium benefits are now on.", Category: categoryAccount}
	if e.Type == events.SubscriptionExpired {
		n.Title, n.Body = "Your plan has expired", "Renew to keep your premium benefits."
	}
	return s.notify(ctx, e, []int64{p.UserID}, n)
}

// ---- social ----

func (s *Service) onFollow(ctx context.Context, e events.Event) error {
	var p events.FollowPayload
	if !decode(e, &p) || p.FollowerID == p.FolloweeID {
		return nil
	}
	name, err := s.displayName(ctx, p.FollowerID)
	if err != nil {
		return err
	}
	return s.notify(ctx, e, []int64{p.FolloweeID}, note{Type: e.Type, Title: "New follower",
		Body: name + " started following you.", Data: map[string]int64{"userId": p.FollowerID}, Category: CategorySocial})
}

func (s *Service) onPost(ctx context.Context, e events.Event) error {
	var p events.PostPayload
	if !decode(e, &p) || p.ActorID == p.AuthorID {
		return nil
	}
	name, err := s.displayName(ctx, p.ActorID)
	if err != nil {
		return err
	}
	n := note{Type: e.Type, Title: "New like", Body: name + " liked your post.", Data: map[string]int64{"postId": p.PostID}, Category: CategorySocial}
	if e.Type == events.PostCommented {
		n.Title, n.Body = "New comment", name+" commented on your post."
	}
	return s.notify(ctx, e, []int64{p.AuthorID}, n)
}

func (s *Service) onConsultation(ctx context.Context, e events.Event) error {
	var p events.ConsultationPayload
	if !decode(e, &p) || p.CoachID == p.PlayerID {
		return nil
	}
	data := map[string]int64{"consultationRequestId": p.RequestID}
	if e.Type == events.ConsultationRequested {
		name, err := s.displayName(ctx, p.PlayerID)
		if err != nil {
			return err
		}
		return s.notify(ctx, e, []int64{p.CoachID}, note{Type: e.Type, Title: "New consultation request",
			Body: name + " sent you a consultation request.", Data: data, Category: CategorySocial})
	}
	// Responded: tell the other party of whoever acted (responderId, else the event actor;
	// the coach when neither is known).
	responder := p.ResponderID
	if responder == 0 {
		responder = e.ActorID
	}
	actor, recipient := p.CoachID, p.PlayerID
	if responder == p.PlayerID {
		actor, recipient = p.PlayerID, p.CoachID
	}
	name, err := s.displayName(ctx, actor)
	if err != nil {
		return err
	}
	verb := "updated your consultation request."
	switch p.Status {
	case "accepted":
		verb = "accepted your consultation request."
	case "declined":
		verb = "declined your consultation request."
	case "cancelled":
		verb = "cancelled the consultation request."
	case "completed":
		verb = "marked the consultation as completed."
	}
	return s.notify(ctx, e, []int64{recipient}, note{Type: e.Type, Title: "Consultation update",
		Body: name + " " + verb, Data: data, Category: CategorySocial})
}

func (s *Service) onCommunityJoin(ctx context.Context, e events.Event) error {
	var p events.CommunityJoinPayload
	if !decode(e, &p) {
		return nil
	}
	conv, ok, err := s.repo.conversation(ctx, p.ConversationID)
	if err != nil || !ok {
		return err
	}
	title := conv.Title
	if title == "" {
		title = "your community"
	}
	data := map[string]int64{"communityId": p.ConversationID, "conversationId": p.ConversationID}
	if e.Type == events.CommunityJoinRequested {
		if p.OwnerID == p.UserID {
			return nil
		}
		name, err := s.displayName(ctx, p.UserID)
		if err != nil {
			return err
		}
		data["userId"] = p.UserID
		return s.notify(ctx, e, []int64{p.OwnerID}, note{Type: e.Type, Title: "New join request",
			Body: name + " asked to join " + quote(title) + ".", Data: data, Category: CategorySocial})
	}
	return s.notify(ctx, e, []int64{p.UserID}, note{Type: e.Type, Title: "You're in",
		Body: "Your request to join " + quote(title) + " was approved.", Data: data, Category: CategorySocial})
}

// ---- organisers and events ----

func (s *Service) onOrganiser(ctx context.Context, e events.Event) error {
	var p events.OrganiserPayload
	if !decode(e, &p) {
		return nil
	}
	n := note{Type: e.Type, Title: "You're approved as an organiser", Body: "You can now create events on Athlix.", Category: CategoryEvents}
	if e.Type == events.OrganiserRejected {
		n.Title, n.Body = "Organiser application not approved", "Open the app to see the details and apply again."
	}
	return s.notify(ctx, e, []int64{p.UserID}, n)
}

func (s *Service) onEventForOrganiser(ctx context.Context, e events.Event) error {
	var p events.EventPayload
	if !decode(e, &p) {
		return nil
	}
	ev, ok, err := s.repo.event(ctx, p.EventID)
	if err != nil || !ok {
		return err
	}
	organiser := p.OrganiserID
	if organiser == 0 {
		organiser = ev.OrganiserID
	}
	t := quote(ev.Title)
	n := note{Type: e.Type, Data: map[string]int64{"eventId": p.EventID}, Category: CategoryEvents}
	switch e.Type {
	case events.EventSubmitted:
		n.Title, n.Body = "Your event is in review", t+" was submitted and is waiting for review."
	case events.EventPublished:
		n.Title, n.Body = "Your event is live", t+" is now published."
	default: // rejected
		n.Title, n.Body = "Your event was not approved", t+" was not approved. Open the app for details."
	}
	return s.notify(ctx, e, []int64{organiser}, n)
}

func (s *Service) onEventCancelled(ctx context.Context, e events.Event) error {
	var p events.EventPayload
	if !decode(e, &p) {
		return nil
	}
	ev, ok, err := s.repo.event(ctx, p.EventID)
	if err != nil || !ok {
		return err
	}
	players, err := s.repo.eventPlayers(ctx, p.EventID)
	if err != nil {
		return err
	}
	organiser := p.OrganiserID
	if organiser == 0 {
		organiser = ev.OrganiserID
	}
	players = without(players, organiser)
	data := map[string]int64{"eventId": p.EventID}
	if err := s.notify(ctx, e, players, note{Type: e.Type, Title: "Event cancelled",
		Body: quote(ev.Title) + " has been cancelled.", Data: data, Category: CategoryEvents}); err != nil {
		return err
	}
	if ev.Status == "removed" {
		return s.notify(ctx, e, []int64{organiser}, note{Type: e.Type, Title: "Your event was removed",
			Body: quote(ev.Title) + " was removed by the Athlix team. Open the app for details.", Data: data, Category: CategoryEvents})
	}
	return nil
}

func (s *Service) onEventUpdated(ctx context.Context, e events.Event) error {
	var p events.EventPayload
	if !decode(e, &p) {
		return nil
	}
	ev, ok, err := s.repo.event(ctx, p.EventID)
	if err != nil || !ok {
		return err
	}
	players, err := s.repo.eventPlayers(ctx, p.EventID)
	if err != nil {
		return err
	}
	return s.notify(ctx, e, without(players, ev.OrganiserID), note{Type: e.Type, Title: "Event updated",
		Body: quote(ev.Title) + " has new details. Take a look.", Data: map[string]int64{"eventId": p.EventID}, Category: CategoryEvents})
}

func (s *Service) onRegistrationForOrganiser(ctx context.Context, e events.Event) error {
	var p events.RegistrationPayload
	if !decode(e, &p) {
		return nil
	}
	ev, ok, err := s.repo.event(ctx, p.EventID)
	if err != nil || !ok {
		return err
	}
	organiser := p.OrganiserID
	if organiser == 0 {
		organiser = ev.OrganiserID
	}
	if organiser == p.UserID {
		return nil
	}
	name, err := s.displayName(ctx, p.UserID)
	if err != nil {
		return err
	}
	t := quote(ev.Title)
	n := note{Type: e.Type, Data: map[string]int64{"eventId": p.EventID, "userId": p.UserID}, Category: CategoryEvents}
	switch {
	case e.Type == events.RegistrationWithdrawn:
		n.Title, n.Body = "Registration withdrawn", name+" withdrew from "+t+"."
	case p.Status == "waitlisted":
		n.Title, n.Body = "New waitlist entry", name+" joined the waitlist for "+t+"."
	default:
		n.Title, n.Body = "New registration", name+" registered for "+t+"."
	}
	return s.notify(ctx, e, []int64{organiser}, n)
}

func (s *Service) onRegistrationForPlayer(ctx context.Context, e events.Event) error {
	var p events.RegistrationPayload
	if !decode(e, &p) {
		return nil
	}
	ev, ok, err := s.repo.event(ctx, p.EventID)
	if err != nil || !ok {
		return err
	}
	t := quote(ev.Title)
	n := note{Type: e.Type, Data: map[string]int64{"eventId": p.EventID}, Category: CategoryEvents}
	if e.Type == events.RegistrationPromoted {
		n.Title, n.Body = "You're in!", "A spot opened up in "+t+". You're now registered."
	} else {
		n.Title = "Payment updated"
		switch p.PaymentStatus {
		case "paid_online", "paid_offline":
			n.Body = "Your payment for " + t + " is marked as received."
		case "waived":
			n.Body = "Your fee for " + t + " was waived."
		default:
			n.Body = "Your payment for " + t + " is marked as not received."
		}
	}
	return s.notify(ctx, e, []int64{p.UserID}, n)
}

func (s *Service) onListing(ctx context.Context, e events.Event) error {
	var p events.ListingPayload
	if !decode(e, &p) {
		return nil
	}
	ev, ok, err := s.repo.event(ctx, p.EventID)
	if err != nil || !ok {
		return err
	}
	organiser := p.OrganiserID
	if organiser == 0 {
		organiser = ev.OrganiserID
	}
	t := quote(ev.Title)
	n := note{Type: e.Type, Title: "Listing fee paid", Body: "We received the listing fee for " + t + ".",
		Data: map[string]int64{"eventId": p.EventID}, Category: CategoryEvents}
	if e.Type == events.ListingRefunded {
		n.Title, n.Body = "Listing fee refunded", fmt.Sprintf("%s of your listing fee for %s was refunded.", rupees(p.AmountPaise), t)
	}
	return s.notify(ctx, e, []int64{organiser}, n)
}

// ---- chat ----

// onMessage pushes a new chat message to offline, unmuted members. It writes no in-app row
// (the conversation list already shows unread counts). The message text is never pushed.
func (s *Service) onMessage(ctx context.Context, e events.Event) error {
	var p events.MessagePayload
	if !decode(e, &p) || p.SenderID == 0 {
		return nil
	}
	conv, ok, err := s.repo.conversation(ctx, p.ConversationID)
	if err != nil || !ok || conv.Status != "active" {
		return err
	}
	kind, status, ok, err := s.repo.message(ctx, p.MessageID)
	if err != nil || !ok || status != "visible" || kind == "system" {
		return err
	}
	members, err := s.repo.unmutedMembers(ctx, p.ConversationID, p.SenderID, s.now())
	if err != nil || len(members) == 0 {
		return err
	}
	offline, err := s.offline(ctx, members)
	if err != nil || len(offline) == 0 {
		return err
	}
	name, err := s.displayName(ctx, p.SenderID)
	if err != nil {
		return err
	}
	n := note{Type: e.Type, Data: map[string]int64{"conversationId": p.ConversationID}, Category: CategoryMessages}
	if conv.Type == "direct" {
		n.Title, n.Body = name, "sent you a message"
		if kind == "image" {
			n.Body = "sent you a photo"
		}
	} else {
		n.Title, n.Body = clip(conv.Title, titleMaxLen), name+": new message"
		if conv.Title == "" {
			n.Title = "New group message"
		}
		if kind == "image" {
			n.Body = name + ": sent a photo"
		}
	}
	var fresh []int64
	var keys []string
	for _, uid := range offline {
		key := dedupeKey(e, uid)
		ok, err := s.claim(ctx, key, dedupeTTL)
		if err != nil {
			s.release(ctx, keys...)
			return err
		}
		if ok {
			fresh = append(fresh, uid)
			keys = append(keys, key)
		}
	}
	if err := s.pushMany(ctx, fresh, n); err != nil {
		s.release(ctx, keys...)
		return err
	}
	return nil
}

// offline filters out users with a live WebSocket connection (chat keeps ws:user:<id> sets).
func (s *Service) offline(ctx context.Context, ids []int64) ([]int64, error) {
	pipe := s.d.Redis.Pipeline()
	cmds := make([]interface{ Val() int64 }, len(ids))
	for i, id := range ids {
		cmds[i] = pipe.SCard(ctx, "ws:user:"+strconv.FormatInt(id, 10))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	var out []int64
	for i, id := range ids {
		if cmds[i].Val() == 0 {
			out = append(out, id)
		}
	}
	return out, nil
}

// ---- helpers ----

func without(ids []int64, drop int64) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != drop {
			out = append(out, id)
		}
	}
	return out
}

func rupees(paise int64) string {
	if paise%100 == 0 {
		return fmt.Sprintf("₹%d", paise/100)
	}
	return fmt.Sprintf("₹%d.%02d", paise/100, paise%100)
}

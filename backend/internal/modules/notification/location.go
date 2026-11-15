package notification

import (
	"context"
	"time"

	"athlix/backend/internal/platform/events"
)

// Location eligibility notices (docs/architecture.md "Location eligibility"). Text names the
// district only; proof details, addresses and rejection reasons never appear.

const (
	screenLocationIdentity   = "location_identity"
	screenEventRegistrations = "event_registrations"
)

// ist is used to show expiry dates the way users read them (no tzdata dependency).
var ist = time.FixedZone("IST", 5*60*60+30*60)

// identityLabel is "Thanjavur native" / "Thanjavur residence" ("Your native" when the district
// is unknown to the places cache).
func (s *Service) identityLabel(ctx context.Context, p events.LocationIdentityPayload) string {
	kind := "residence"
	if p.IdentityType == "native" {
		kind = "native"
	}
	if name := s.d.Eligibility.Places().DistrictName(ctx, p.DistrictID); name != "" {
		return clip(name, 40) + " " + kind
	}
	return kind
}

func (s *Service) onLocationIdentityChanged(ctx context.Context, e events.Event) error {
	var p events.LocationIdentityPayload
	if !decode(e, &p) || p.UserID == 0 {
		return nil
	}
	label := s.identityLabel(ctx, p)
	n := note{Type: e.Type, Links: map[string]string{"screen": screenLocationIdentity}, Category: CategoryEvents}
	switch p.Status {
	case "verified":
		n.Title, n.Body = "Location identity verified", "Your "+label+" identity is verified."
	case "rejected":
		n.Title, n.Body = "Location identity needs a new document",
			"We couldn't verify your "+label+" identity. Please upload a new document."
		if p.RejectionCode != "" {
			n.Links["rejectionCode"] = clip(p.RejectionCode, 48)
		}
	case "expired":
		n.Title, n.Body = "Residence expired", "Your "+label+" identity expired. Renew it to keep joining district events."
	case "revoked":
		n.Title, n.Body = "Location identity revoked", "Your "+label+" identity was revoked. Open the app for details."
	default: // pending, superseded, withdrawn: the user caused it or nothing changed for them
		return nil
	}
	return s.notify(ctx, e, []int64{p.UserID}, n)
}

func (s *Service) onLocationIdentityExpiring(ctx context.Context, e events.Event) error {
	var p events.LocationIdentityPayload
	if !decode(e, &p) || p.UserID == 0 {
		return nil
	}
	body := "Your " + s.identityLabel(ctx, p) + " identity expires soon. Renew now."
	if t, err := time.Parse(time.RFC3339, p.ExpiresAt); err == nil {
		body = "Your " + s.identityLabel(ctx, p) + " identity expires on " + t.In(ist).Format("2 Jan 2006") + ". Renew now."
	}
	return s.notify(ctx, e, []int64{p.UserID}, note{Type: e.Type, Title: "Residence expiring",
		Body: body, Links: map[string]string{"screen": screenLocationIdentity}, Category: CategoryEvents})
}

// onEligibilityFlagged tells both sides: the organiser reviews, the player knows nothing was
// cancelled. Two notify calls so each gets its own text (dedupe is per recipient).
func (s *Service) onEligibilityFlagged(ctx context.Context, e events.Event) error {
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
	t := quote(ev.Title)
	if organiser != 0 && organiser != p.UserID {
		name, err := s.displayName(ctx, p.UserID)
		if err != nil {
			return err
		}
		if err := s.notify(ctx, e, []int64{organiser}, note{Type: e.Type, Title: "Registration needs review",
			Body:  name + "'s location identity for " + t + " changed. Please review the registration. It has not been cancelled.",
			Data:  map[string]int64{"eventId": p.EventID, "userId": p.UserID},
			Links: map[string]string{"screen": screenEventRegistrations}, Category: CategoryEvents}); err != nil {
			return err
		}
	}
	if p.UserID == 0 {
		return nil
	}
	return s.notify(ctx, e, []int64{p.UserID}, note{Type: e.Type, Title: "Registration under review",
		Body: "Your location identity changed, so the organiser of " + t + " will review your registration. It has not been cancelled.",
		Data: map[string]int64{"eventId": p.EventID}, Links: map[string]string{"screen": screenLocationIdentity}, Category: CategoryEvents})
}

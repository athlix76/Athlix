package requirements

import (
	"context"
	"testing"
)

func TestStatic(t *testing.T) {
	c := NewStatic(map[string]bool{"organiser/organiser_approval/create_event": true, "coach/coach_pro_subscription/offer_consultation": false})
	ctx := context.Background()
	if !c.Required(ctx, "organiser", OrganiserApproval, GateCreateEvent) {
		t.Fatal("organiser approval should be required")
	}
	if c.Required(ctx, "coach", CoachProSubscription, GateOfferConsultation) {
		t.Fatal("coach pro should not be required")
	}
	if !c.RequiredForAny(ctx, []string{"player", "organiser"}, OrganiserApproval, GateCreateEvent) {
		t.Fatal("any should match organiser")
	}
}

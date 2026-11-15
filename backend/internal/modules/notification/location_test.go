package notification

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/eligibility"
	"athlix/backend/internal/platform/events"
)

func identityEvent(typ, status string, extra func(*events.LocationIdentityPayload)) events.Event {
	p := events.LocationIdentityPayload{IdentityID: 3, UserID: 8, IdentityType: "native", DistrictID: eligibility.TestDistrictThanj, Status: status}
	if extra != nil {
		extra(&p)
	}
	return events.New(typ, 0, p)
}

func TestLocationIdentityChangedTexts(t *testing.T) {
	cases := []struct {
		status, idType, title, body, data string
		code                              string
	}{
		{"verified", "native", "Location identity verified", "Your Thanjavur native identity is verified.", `{"screen":"location_identity"}`, ""},
		{"rejected", "residence", "Location identity needs a new document",
			"We couldn't verify your Thanjavur residence identity. Please upload a new document.",
			`{"rejectionCode":"unclear_document","screen":"location_identity"}`, "unclear_document"},
		{"expired", "residence", "Residence expired",
			"Your Thanjavur residence identity expired. Renew it to keep joining district events.", `{"screen":"location_identity"}`, ""},
		{"revoked", "native", "Location identity revoked",
			"Your Thanjavur native identity was revoked. Open the app for details.", `{"screen":"location_identity"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			_, mock, fake, d := setup(t, nil)
			e := identityEvent(events.LocationIdentityChanged, tc.status, func(p *events.LocationIdentityPayload) {
				p.IdentityType, p.RejectionCode = tc.idType, tc.code
			})
			expectInsert(mock, 8, events.LocationIdentityChanged, tc.title, tc.body, tc.data)
			expectPrefs(mock, nil, int64(8))
			expectDevices(mock, devRows().AddRow(1, 8, "tok-8"), int64(8))
			dispatch(t, d, e)
			dispatch(t, d, e) // redelivery: no second row, no second push

			got := fake.Messages()
			if len(got) != 1 || got[0].Title != tc.title || got[0].Data["screen"] != "location_identity" || got[0].Data["rejectionCode"] != tc.code {
				t.Fatalf("pushes = %+v", got)
			}
		})
	}
}

func TestLocationIdentityChangedSilentStatuses(t *testing.T) {
	_, _, fake, d := setup(t, nil) // any SQL would fail the test: none is expected
	for _, st := range []string{"pending", "superseded", "withdrawn"} {
		dispatch(t, d, identityEvent(events.LocationIdentityChanged, st, nil))
	}
	if len(fake.Messages()) != 0 {
		t.Fatal("unexpected push")
	}
}

func TestLocationIdentityChangedRespectsEventsPref(t *testing.T) {
	_, mock, fake, d := setup(t, nil)
	expectInsert(mock, 8, events.LocationIdentityChanged, "Location identity verified", "Your Thanjavur native identity is verified.", `{"screen":"location_identity"}`)
	expectPrefs(mock, sqlmock.NewRows([]string{"user_id", "notification_prefs"}).AddRow(8, []byte(`{"events":false}`)), int64(8))
	dispatch(t, d, identityEvent(events.LocationIdentityChanged, "verified", nil))
	if len(fake.Messages()) != 0 {
		t.Fatal("push sent although events is off")
	}
}

func TestLocationIdentityExpiring(t *testing.T) {
	_, mock, fake, d := setup(t, nil)
	e := identityEvent(events.LocationIdentityExpiring, "verified", func(p *events.LocationIdentityPayload) {
		p.IdentityType, p.ExpiresAt = "residence", "2026-12-01T20:00:00Z" // 2 Dec in IST
	})
	expectInsert(mock, 8, events.LocationIdentityExpiring, "Residence expiring",
		"Your Thanjavur residence identity expires on 2 Dec 2026. Renew now.", `{"screen":"location_identity"}`)
	expectPrefs(mock, nil, int64(8))
	expectDevices(mock, devRows().AddRow(1, 8, "tok-8"), int64(8))
	dispatch(t, d, e)
	dispatch(t, d, e)
	if len(fake.Messages()) != 1 {
		t.Fatalf("pushes = %+v", fake.Messages())
	}

	// events pref off: row written, no push.
	_, mock2, fake2, d2 := setup(t, nil)
	expectInsert(mock2, 8, events.LocationIdentityExpiring, "Residence expiring",
		"Your Thanjavur residence identity expires on 2 Dec 2026. Renew now.", `{"screen":"location_identity"}`)
	expectPrefs(mock2, sqlmock.NewRows([]string{"user_id", "notification_prefs"}).AddRow(8, []byte(`{"events":false}`)), int64(8))
	dispatch(t, d2, e)
	if len(fake2.Messages()) != 0 {
		t.Fatal("push sent although events is off")
	}
}

func TestEligibilityFlaggedNotifiesOrganiserAndPlayer(t *testing.T) {
	_, mock, fake, d := setup(t, nil)
	e := events.New(events.RegistrationEligibilityFlagged, 0, events.RegistrationPayload{
		RegistrationID: 1, EventID: 5, UserID: 8, OrganiserID: 50, Status: "registered", EligibilityFlag: "identity_expired"})

	expectEvent(mock, 5, "City Cup", "published", 50)
	expectName(mock, 8, "Ann")
	expectInsert(mock, 50, events.RegistrationEligibilityFlagged, "Registration needs review",
		"Ann's location identity for “City Cup” changed. Please review the registration. It has not been cancelled.",
		`{"eventId":5,"screen":"event_registrations","userId":8}`)
	expectPrefs(mock, nil, int64(50))
	expectDevices(mock, devRows().AddRow(1, 50, "tok-50"), int64(50))
	expectInsert(mock, 8, events.RegistrationEligibilityFlagged, "Registration under review",
		"Your location identity changed, so the organiser of “City Cup” will review your registration. It has not been cancelled.",
		`{"eventId":5,"screen":"location_identity"}`)
	expectPrefs(mock, sqlmock.NewRows([]string{"user_id", "notification_prefs"}).AddRow(8, []byte(`{"events":false}`)), int64(8))
	dispatch(t, d, e)

	// Redelivery: the event and name are re-read, but nothing is written or pushed again.
	expectEvent(mock, 5, "City Cup", "published", 50)
	expectName(mock, 8, "Ann")
	dispatch(t, d, e)

	got := fake.Messages()
	if len(got) != 1 || got[0].Token != "tok-50" || got[0].Data["screen"] != "event_registrations" || got[0].Data["eventId"] != "5" {
		t.Fatalf("pushes = %+v (player has events off)", got)
	}
}

func TestLocationHandlersRegistered(t *testing.T) {
	m, _, _, _ := setup(t, nil)
	h := m.EventHandlers()
	for _, typ := range []string{events.LocationIdentityChanged, events.LocationIdentityExpiring, events.RegistrationEligibilityFlagged} {
		if h[typ] == nil {
			t.Errorf("no handler for %s", typ)
		}
	}
}

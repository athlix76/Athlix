package guardian

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/modules/user/usertest"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/testutil"
)

func (e *env) expectGranted(t *testing.T, consentID int64) {
	t.Helper()
	enc, err := e.d.Crypto.EncryptString(guardianE164)
	if err != nil {
		t.Fatal(err)
	}
	e.mock.ExpectQuery(`SELECT id, guardian_mobile_enc FROM guardian_consents WHERE user_id = \? AND status = 'granted'`).
		WithArgs(uid).WillReturnRows(sqlmock.NewRows([]string{"id", "guardian_mobile_enc"}).AddRow(consentID, enc))
}

func (e *env) expectRevoke(consentID int64, reason string) {
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(`SELECT id FROM guardian_consents WHERE user_id = \? AND status = 'granted' ORDER BY id DESC LIMIT 1 FOR UPDATE`).
		WithArgs(uid).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(consentID))
	e.mock.ExpectExec(`UPDATE guardian_consents SET status = 'revoked', revoked_at = \?, revoked_reason = \? WHERE id = \? AND user_id = \? AND status = 'granted'`).
		WithArgs(fixedNow, reason, consentID, uid).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`UPDATE users SET guardian_consent_status = 'revoked', onboarding_step = IF\(is_minor = 1, 'guardian', onboarding_step\) WHERE id = \?`).
		WithArgs(uid).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestWithdrawSendsCodeToGuardianAndRevokes(t *testing.T) {
	e := newEnv(t, nil)
	var revoked []int64
	e.d.Dispatcher.On(events.GuardianConsentRevoked, func(_ context.Context, ev events.Event) error {
		var p events.UserPayload
		_ = ev.Decode(&p)
		revoked = append(revoked, p.UserID)
		return nil
	})
	e.expectGranted(t, 60)
	w := testutil.Request(e.r, http.MethodPost, "/v1/me/guardian/withdraw/request", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("request %d %s", w.Code, w.Body)
	}
	var tk Ticket
	_ = json.Unmarshal(decode(t, w).Data, &tk)
	if tk.ConsentID != 60 || tk.GuardianMobileMasked == "" {
		t.Fatalf("ticket = %+v", tk)
	}
	// The code goes to the guardian, never to the minor's own number.
	if len(e.sms.Sent) != 1 || e.sms.Sent[0].Mobile != "919123456789" {
		t.Fatalf("sms = %+v", e.sms.Sent)
	}

	e.expectConsent(60, "granted", false)
	if w := testutil.Request(e.r, http.MethodPost, "/v1/me/guardian/withdraw/verify", map[string]any{"consentId": 60, "otp": "0000x"}, nil); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong code: %d", w.Code)
	}
	e.expectConsent(60, "granted", false)
	e.expectRevoke(60, reasonGuardianWithdrew)
	e.mock.ExpectCommit()
	usertest.ExpectMe(e.mock, usertest.MeRow{ID: uid, PrimaryRole: "player", Roles: []string{"player"}, Step: "guardian", IsMinor: true, Guardian: "revoked"})
	w = testutil.Request(e.r, http.MethodPost, "/v1/me/guardian/withdraw/verify", map[string]any{"consentId": 60, "otp": e.sms.Sent[0].OTP}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("verify %d %s", w.Code, w.Body)
	}
	if len(revoked) != 1 || revoked[0] != uid {
		t.Fatalf("events = %v", revoked)
	}
	if err := e.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWithdrawWithoutGrantedConsentIs409(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectQuery(`SELECT id, guardian_mobile_enc FROM guardian_consents`).WithArgs(uid).WillReturnRows(sqlmock.NewRows([]string{"id", "guardian_mobile_enc"}))
	if w := testutil.Request(e.r, http.MethodPost, "/v1/me/guardian/withdraw/request", nil, nil); w.Code != http.StatusConflict {
		t.Fatalf("code %d", w.Code)
	}
	// Another user's consent id is 404.
	e.expectConsent(99, "", false)
	if w := testutil.Request(e.r, http.MethodPost, "/v1/me/guardian/withdraw/verify", map[string]any{"consentId": 99, "otp": "1234"}, nil); w.Code != http.StatusNotFound {
		t.Fatalf("other consent: %d", w.Code)
	}
}

func TestAdminRevoke(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	d.Now = func() time.Time { return fixedNow }
	e := &env{d: d, mock: mock}
	m := New(d)
	r, routes := testutil.AdminEngine(3, []string{"users.write"})
	m.Routes(routes)

	if w := testutil.Request(r, http.MethodPost, "/v1/admin/users/7/guardian-consent/revoke", map[string]any{"reason": " "}, nil); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty reason: %d", w.Code)
	}
	e.expectRevoke(61, "Guardian asked support by email")
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(3, "guardian.consent_revoke", "user", "7", nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	w := testutil.Request(r, http.MethodPost, "/v1/admin/users/7/guardian-consent/revoke", map[string]any{"reason": "Guardian asked support by email"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke %d %s", w.Code, w.Body)
	}
	var out Revoked
	_ = json.Unmarshal(decode(t, w).Data, &out)
	if out.ConsentID != 61 || out.GuardianConsentStatus != "revoked" {
		t.Fatalf("out = %+v", out)
	}

	r2, routes2 := testutil.AdminEngine(3, []string{"users.read"})
	New(d).Routes(routes2)
	if w := testutil.Request(r2, http.MethodPost, "/v1/admin/users/7/guardian-consent/revoke", map[string]any{"reason": "x"}, nil); w.Code != http.StatusForbidden {
		t.Fatalf("without users.write: %d", w.Code)
	}
}

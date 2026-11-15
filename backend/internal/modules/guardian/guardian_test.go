package guardian

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/user/usertest"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/testutil"
)

const (
	uid          = int64(7)
	ownMobile    = "+919876543210"
	guardianNum  = "91234 56789"
	guardianE164 = "+919123456789"
)

var fixedNow = time.Date(2026, 6, 15, 6, 0, 0, 0, time.UTC)

type body struct {
	Data  json.RawMessage `json:"data"`
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func decode(t *testing.T, w *httptest.ResponseRecorder) body {
	t.Helper()
	var b body
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
			t.Fatalf("decode %q: %v", w.Body.String(), err)
		}
	}
	return b
}

type env struct {
	d       *app.Deps
	mock    sqlmock.Sqlmock
	m       *Module
	r       *gin.Engine
	sms     *testutil.RecordingSMS
	granted []int64
}

func newEnv(t *testing.T, cfg map[string]any) *env {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return fixedNow }
	e := &env{d: d, mock: mock, sms: d.SMS.(*testutil.RecordingSMS)}
	d.Dispatcher.On(events.GuardianConsentGranted, func(_ context.Context, ev events.Event) error {
		var p events.UserPayload
		_ = ev.Decode(&p)
		e.granted = append(e.granted, p.UserID)
		return nil
	})
	e.m = New(d)
	u := testutil.VerifiedUser(uid)
	u.OnboardingStep = "guardian"
	r, routes := testutil.UserEngine(u)
	e.m.Routes(routes)
	e.r = r
	return e
}

func validReq() map[string]any {
	return map[string]any{"guardianName": "Meena Kulkarni", "guardianMobile": guardianNum, "relation": "mother", "policyDocumentId": 5}
}

func (e *env) expectUser(verify string, isMinor bool, guardian string) {
	e.mock.ExpectQuery(`SELECT verification_status, is_minor, guardian_consent_status, mobile_hash FROM users WHERE id = \? AND status <> 'deleted'`).
		WithArgs(uid).WillReturnRows(sqlmock.NewRows([]string{"verification_status", "is_minor", "guardian_consent_status", "mobile_hash"}).
		AddRow(verify, isMinor, guardian, e.d.Crypto.Hash("mobile", ownMobile)))
}

func (e *env) expectPolicy(active bool) {
	rows := sqlmock.NewRows([]string{"id"})
	if active {
		rows.AddRow(5)
	}
	e.mock.ExpectQuery(`SELECT id FROM policy_documents WHERE id = \? AND doc_type = 'guardian_consent' AND active = 1`).WithArgs(int64(5)).WillReturnRows(rows)
}

// capturedArg records an argument for later checks.
type capturedArg struct{ v any }

func (c *capturedArg) Match(v driver.Value) bool { c.v = v; return true }

func (e *env) expectInsert(consentID int64, name, mobile, hash *capturedArg) {
	e.mock.ExpectBegin()
	e.mock.ExpectExec(`UPDATE guardian_consents SET status = 'expired' WHERE user_id = \? AND status = 'pending'`).WithArgs(uid).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`INSERT INTO guardian_consents \(user_id, guardian_name_enc, guardian_mobile_enc, guardian_mobile_hash, guardian_mobile_last4, relation, status, policy_document_id, key_version\) VALUES`).
		WithArgs(uid, name, mobile, hash, "6789", "mother", int64(5), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(consentID, 1))
	e.mock.ExpectExec(`UPDATE users SET guardian_consent_status = 'pending' WHERE id = \? AND guardian_consent_status <> 'granted'`).WithArgs(uid).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
}

func (e *env) request(t *testing.T, consentID int64) Ticket {
	t.Helper()
	e.expectUser("verified", true, "not_required")
	e.expectPolicy(true)
	e.expectInsert(consentID, &capturedArg{}, &capturedArg{}, &capturedArg{})
	w := testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/request", validReq(), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("request %d %s", w.Code, w.Body)
	}
	var tk Ticket
	_ = json.Unmarshal(decode(t, w).Data, &tk)
	return tk
}

func (e *env) expectConsent(consentID int64, status string, forUpdate bool) {
	q := `SELECT status FROM guardian_consents WHERE id = \? AND user_id = \?`
	if forUpdate {
		q += ` FOR UPDATE`
	} else {
		q += `$`
	}
	rows := sqlmock.NewRows([]string{"status"})
	if status != "" {
		rows.AddRow(status)
	}
	e.mock.ExpectQuery(q).WithArgs(consentID, uid).WillReturnRows(rows)
}

func verify(e *env, consentID int64, code string) *httptest.ResponseRecorder {
	return testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/verify", map[string]any{"consentId": consentID, "otp": code}, nil)
}

// ---- request ----

func TestRequestHappyPath(t *testing.T) {
	e := newEnv(t, map[string]any{"guardian.consent_otp_ttl_seconds": 120, "auth.otp.resend_cooldown_seconds": 45, "auth.otp.length": 4})
	name, mobile, hash := &capturedArg{}, &capturedArg{}, &capturedArg{}
	e.expectUser("verified", true, "not_required")
	e.expectPolicy(true)
	e.expectInsert(31, name, mobile, hash)
	w := testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/request", validReq(), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	var tk Ticket
	_ = json.Unmarshal(decode(t, w).Data, &tk)
	if tk != (Ticket{ConsentID: 31, GuardianMobileMasked: "******6789", ExpiresInSeconds: 120, ResendAfterSeconds: 45}) {
		t.Fatalf("ticket = %+v", tk)
	}
	if strings.Contains(w.Body.String(), "9123456789") || strings.Contains(w.Body.String(), "Meena") {
		t.Fatal("response leaks guardian PII")
	}
	// Stored encrypted, with the same HMAC purpose as users.mobile_hash.
	if got, _ := e.d.Crypto.DecryptString(name.v.([]byte)); got != "Meena Kulkarni" {
		t.Fatalf("name ciphertext decrypts to %q", got)
	}
	if got, _ := e.d.Crypto.DecryptString(mobile.v.([]byte)); got != guardianE164 {
		t.Fatalf("mobile ciphertext decrypts to %q", got)
	}
	if string(hash.v.([]byte)) != string(e.d.Crypto.Hash("mobile", guardianE164)) {
		t.Fatal("mobile hash")
	}
	// The OTP goes to the guardian, with the guardian template and configured length.
	if len(e.sms.Sent) != 1 || e.sms.Sent[0].Template != "guardian_otp" || e.sms.Sent[0].Mobile != "919123456789" || len(e.sms.Sent[0].OTP) != 4 {
		t.Fatalf("sms = %+v", e.sms.Sent)
	}
	// The challenge holds only the code hash and the owner id, with the configured TTL.
	ctx := context.Background()
	vals := e.d.Redis.HGetAll(ctx, keyChallenge+"31").Val()
	for _, v := range vals {
		if strings.Contains(v, "9123456789") || v == e.sms.Sent[0].OTP {
			t.Fatalf("challenge stores raw data: %v", vals)
		}
	}
	if ttl := e.d.Redis.TTL(ctx, keyChallenge+"31").Val(); ttl <= 0 || ttl > 120*time.Second {
		t.Fatalf("ttl = %v", ttl)
	}

	// Resend within the cooldown is refused before any write.
	e.expectUser("verified", true, "pending")
	e.expectPolicy(true)
	if w := testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/request", validReq(), nil); w.Code != http.StatusTooManyRequests {
		t.Fatalf("resend: %d", w.Code)
	}
}

func TestRequestValidation(t *testing.T) {
	e := newEnv(t, nil)
	cases := map[string]map[string]any{
		"guardianName":     {"guardianName": "X", "guardianMobile": guardianNum, "relation": "mother", "policyDocumentId": 5},
		"guardianMobile":   {"guardianName": "Meena", "guardianMobile": "12345", "relation": "mother", "policyDocumentId": 5},
		"relation":         {"guardianName": "Meena", "guardianMobile": guardianNum, "relation": "aunt", "policyDocumentId": 5},
		"policyDocumentId": {"guardianName": "Meena", "guardianMobile": guardianNum, "relation": "mother"},
	}
	for field, in := range cases {
		w := testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/request", in, nil)
		if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields[field] == "" {
			t.Errorf("%s: %d %s", field, w.Code, w.Body)
		}
	}
	long := validReq()
	long["guardianName"] = strings.Repeat("a", 81)
	if w := testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/request", long, nil); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("long name: %d", w.Code)
	}
}

func TestRequestOwnMobileAndPolicy(t *testing.T) {
	e := newEnv(t, nil)
	own := validReq()
	own["guardianMobile"] = "098765 43210" // same number as the minor, other formatting
	e.expectUser("verified", true, "not_required")
	w := testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/request", own, nil)
	if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["guardianMobile"] == "" {
		t.Fatalf("own mobile: %d %s", w.Code, w.Body)
	}
	e.expectUser("verified", true, "not_required")
	e.expectPolicy(false)
	w = testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/request", validReq(), nil)
	if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["policyDocumentId"] == "" {
		t.Fatalf("inactive policy: %d %s", w.Code, w.Body)
	}
	if len(e.sms.Sent) != 0 {
		t.Fatal("no SMS on rejected requests")
	}
}

func TestRequestStateChecks(t *testing.T) {
	for _, tc := range []struct {
		name, verify, guardian string
		minor                  bool
	}{
		{"adult", "verified", "not_required", false},
		{"not verified", "pending", "not_required", true},
		{"already granted", "verified", "granted", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, nil)
			e.expectUser(tc.verify, tc.minor, tc.guardian)
			w := testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/request", validReq(), nil)
			if w.Code != http.StatusConflict || decode(t, w).Error.Code != "INVALID_STATE" {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
}

func TestRequestHourlyLimitPerGuardianMobile(t *testing.T) {
	e := newEnv(t, map[string]any{"auth.otp.max_per_hour": 1, "auth.otp.resend_cooldown_seconds": 0})
	e.request(t, 40)
	e.expectUser("verified", true, "pending")
	e.expectPolicy(true)
	if w := testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/request", validReq(), nil); w.Code != http.StatusTooManyRequests {
		t.Fatalf("second within the hour: %d", w.Code)
	}
}

type failingSMS struct{}

func (failingSMS) SendOTP(context.Context, string, string, string) error {
	return errors.New("gateway down")
}

func TestRequestSMSFailureIs502(t *testing.T) {
	e := newEnv(t, nil)
	e.d.SMS = failingSMS{}
	e.m = New(e.d)
	r, routes := testutil.UserEngine(testutil.VerifiedUser(uid))
	e.m.Routes(routes)
	e.r = r
	e.expectUser("verified", true, "not_required")
	e.expectPolicy(true)
	e.expectInsert(41, &capturedArg{}, &capturedArg{}, &capturedArg{})
	if w := testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/request", validReq(), nil); w.Code != http.StatusBadGateway {
		t.Fatalf("code %d", w.Code)
	}
}

// ---- verify ----

func expectGrant(e *env, consentID int64) {
	e.mock.ExpectBegin()
	e.expectConsent(consentID, "pending", true)
	e.mock.ExpectExec(`UPDATE guardian_consents SET status = 'granted', granted_at = \? WHERE id = \? AND user_id = \?`).WithArgs(fixedNow, consentID, uid).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`UPDATE users SET guardian_consent_status = 'granted' WHERE id = \?`).WithArgs(uid).WillReturnResult(sqlmock.NewResult(0, 1))
	usertest.ExpectAdvance(e.mock, uid, usertest.AdvanceRow{PrimaryRole: "player", Step: "guardian", Verify: "verified", IsMinor: true, Guardian: "granted", Roles: []string{"player"}}, "profile")
	e.mock.ExpectCommit()
	usertest.ExpectMe(e.mock, usertest.MeRow{ID: uid, PrimaryRole: "player", Roles: []string{"player"}, Step: "profile", IsMinor: true, Guardian: "granted"})
}

func TestVerifyHappyPath(t *testing.T) {
	e := newEnv(t, nil)
	tk := e.request(t, 50)
	code := e.sms.Sent[0].OTP
	e.expectConsent(50, "pending", false)
	expectGrant(e, 50)
	w := verify(e, tk.ConsentID, code)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	var me struct {
		OnboardingStep        string `json:"onboardingStep"`
		GuardianConsentStatus string `json:"guardianConsentStatus"`
	}
	_ = json.Unmarshal(decode(t, w).Data, &me)
	if me.OnboardingStep != "profile" || me.GuardianConsentStatus != "granted" {
		t.Fatalf("me = %+v", me)
	}
	if len(e.granted) != 1 || e.granted[0] != uid {
		t.Fatalf("events = %v", e.granted)
	}
	// The code is single use.
	e.expectConsent(50, "granted", false)
	if w := verify(e, 50, code); w.Code != http.StatusConflict {
		t.Fatalf("reuse: %d", w.Code)
	}
}

func TestVerifyWrongAndBurntCode(t *testing.T) {
	e := newEnv(t, map[string]any{"auth.otp.max_attempts": 2})
	e.request(t, 51)
	e.expectConsent(51, "pending", false)
	w := verify(e, 51, "000000x")
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(decode(t, w).Error.Fields["otp"], "incorrect") {
		t.Fatalf("wrong: %d %s", w.Code, w.Body)
	}
	e.expectConsent(51, "pending", false)
	w = verify(e, 51, "000000x")
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(decode(t, w).Error.Fields["otp"], "expired") {
		t.Fatalf("burnt: %d %s", w.Code, w.Body)
	}
	// Burnt: even the right code no longer works.
	e.expectConsent(51, "pending", false)
	if w := verify(e, 51, e.sms.Sent[0].OTP); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("after burn: %d", w.Code)
	}
	if len(e.granted) != 0 {
		t.Fatal("no event without a grant")
	}
}

func TestVerifyOtherUsersConsentIs404(t *testing.T) {
	e := newEnv(t, nil)
	e.request(t, 52)
	e.expectConsent(52, "", false) // scoped query: the consent is not the caller's
	w := verify(e, 52, e.sms.Sent[0].OTP)
	if w.Code != http.StatusNotFound || decode(t, w).Error.Code != "NOT_FOUND" {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	if e.d.Redis.Exists(context.Background(), keyChallenge+"52").Val() != 1 {
		t.Fatal("a foreign attempt must not consume the challenge")
	}
}

func TestVerifyValidation(t *testing.T) {
	e := newEnv(t, nil)
	w := testutil.Request(e.r, http.MethodPost, "/v1/onboarding/guardian/verify", map[string]any{}, nil)
	f := decode(t, w).Error.Fields
	if w.Code != http.StatusUnprocessableEntity || f["consentId"] == "" || f["otp"] == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestDevFixedOTPLocalOnly(t *testing.T) {
	e := newEnv(t, nil)
	e.d.Cfg.DevFixedOTP = "424242"
	e.request(t, 60)
	if len(e.sms.Sent) != 0 {
		t.Fatal("fixed OTP sends no SMS")
	}
	e.expectConsent(60, "pending", false)
	expectGrant(e, 60)
	if w := verify(e, 60, "424242"); w.Code != http.StatusOK {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

// ---- events ----

func TestUserDeletedWipeIsIdempotent(t *testing.T) {
	e := newEnv(t, nil)
	h := e.m.EventHandlers()[events.UserDeleted]
	wipe := `UPDATE guardian_consents SET guardian_name_enc = \?, guardian_mobile_enc = \?, guardian_mobile_hash = \?, guardian_mobile_last4 = '0000'`
	e.mock.ExpectExec(wipe).WithArgs([]byte{}, []byte{}, sqlmock.AnyArg(), uid).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(wipe).WithArgs([]byte{}, []byte{}, sqlmock.AnyArg(), uid).WillReturnResult(sqlmock.NewResult(0, 0))
	ev := events.New(events.UserDeleted, uid, events.UserPayload{UserID: uid})
	for i := 0; i < 2; i++ {
		if err := h(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := h(context.Background(), events.New(events.UserDeleted, 0, events.UserPayload{})); err != nil {
		t.Fatal("malformed payload is dropped")
	}
	e.mock.ExpectExec(wipe).WillReturnError(errors.New("db down"))
	if err := h(context.Background(), ev); err == nil {
		t.Fatal("db errors are returned for retry")
	}
}

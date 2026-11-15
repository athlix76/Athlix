package verification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/user/usertest"
	"athlix/backend/internal/platform/config"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/secrets"
	"athlix/backend/internal/testutil"
)

// fixedNow is 2026-06-15 11:30 IST.
var fixedNow = time.Date(2026, 6, 15, 6, 0, 0, 0, time.UTC)

const uid = int64(1)

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
	d      *app.Deps
	mock   sqlmock.Sqlmock
	m      *Module
	r      *gin.Engine
	mu     sync.Mutex
	events []events.VerificationPayload
}

func newEnv(t *testing.T, cfg map[string]any) *env {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return fixedNow }
	e := &env{d: d, mock: mock}
	d.Dispatcher.On(events.VerificationCompleted, func(_ context.Context, ev events.Event) error {
		var p events.VerificationPayload
		if err := ev.Decode(&p); err != nil {
			return err
		}
		e.mu.Lock()
		e.events = append(e.events, p)
		e.mu.Unlock()
		return nil
	})
	e.m = New(d)
	u := testutil.VerifiedUser(uid)
	u.OnboardingStep, u.VerificationStatus = "verification", "unverified"
	r, routes := testutil.UserEngine(u)
	e.m.Routes(routes)
	e.r = r
	return e
}

func (e *env) published(t *testing.T) []events.VerificationPayload {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]events.VerificationPayload(nil), e.events...)
}

// ---- sqlmock helpers ----

const userQ = `SELECT verification_status, is_minor, primary_role, display_name FROM users WHERE id = \? AND status <> 'deleted'`

func expectUser(mock sqlmock.Sqlmock, forUpdate bool, verify string, isMinor bool, role, name any) {
	q := userQ + `$`
	if forUpdate {
		q = userQ + ` FOR UPDATE`
	}
	mock.ExpectQuery(q).WithArgs(uid).WillReturnRows(
		sqlmock.NewRows([]string{"verification_status", "is_minor", "primary_role", "display_name"}).AddRow(verify, isMinor, role, name))
}

func expectNoUser(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(userQ + `$`).WithArgs(uid).WillReturnRows(sqlmock.NewRows([]string{"verification_status", "is_minor", "primary_role", "display_name"}))
}

func expectOpenAttempt(mock sqlmock.Sqlmock, provider string, ref any, attemptID int64) {
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE identity_verifications SET status = 'expired' WHERE user_id = \? AND status = 'initiated'`).WithArgs(uid).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO identity_verifications \(user_id, provider, provider_ref, status\) VALUES \(\?, \?, \?, 'initiated'\)`).
		WithArgs(uid, provider, ref).WillReturnResult(sqlmock.NewResult(attemptID, 1))
	mock.ExpectExec(`UPDATE users SET verification_status = 'pending' WHERE id = \? AND verification_status <> 'verified'`).WithArgs(uid).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectLockOpen(mock sqlmock.Sqlmock, attemptID int64, provider string) {
	mock.ExpectQuery(`SELECT status, provider FROM identity_verifications WHERE id = \? AND user_id = \? FOR UPDATE`).
		WithArgs(attemptID, uid).WillReturnRows(sqlmock.NewRows([]string{"status", "provider"}).AddRow("initiated", provider))
	expectUser(mock, true, "pending", false, "player", "Asha K")
}

func expectSubject(mock sqlmock.Sqlmock, owner int64) {
	rows := sqlmock.NewRows([]string{"user_id"})
	if owner != 0 {
		rows.AddRow(owner)
	}
	mock.ExpectQuery(`SELECT user_id FROM identity_verifications WHERE subject_hash = \? LIMIT 1`).WithArgs(sqlmock.AnyArg()).WillReturnRows(rows)
}

func expectMarkFailed(mock sqlmock.Sqlmock, attemptID int64, reason string) {
	mock.ExpectExec(`UPDATE identity_verifications SET status = 'failed', failure_reason = \? WHERE id = \?`).WithArgs(reason, attemptID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE users SET verification_status = \?, is_minor = \? WHERE id = \?`).WithArgs("failed", false, uid).WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectMarkVerified(mock sqlmock.Sqlmock, attemptID int64, last4, gender string) *sqlmock.ExpectedExec {
	return mock.ExpectExec(`UPDATE identity_verifications SET status = 'verified', subject_hash = \?, aadhaar_last4 = \?, name_enc = \?, dob_enc = \?, gender = \?, key_version = \?, failure_reason = NULL, verified_at = \? WHERE id = \?`).
		WithArgs(sqlmock.AnyArg(), last4, sqlmock.AnyArg(), sqlmock.AnyArg(), gender, sqlmock.AnyArg(), fixedNow, attemptID)
}

// expectVerified expects finish() for a successful identity.
func expectVerified(mock sqlmock.Sqlmock, attemptID int64, provider, last4, gender string, isMinor bool, birthYear int) {
	mock.ExpectBegin()
	expectLockOpen(mock, attemptID, provider)
	expectUser(mock, false, "pending", false, "player", "Asha K")
	expectSubject(mock, 0)
	expectMarkVerified(mock, attemptID, last4, gender).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE users SET verification_status = 'verified', is_minor = \?, guardian_consent_status = IF\(\? AND guardian_consent_status = 'not_required', 'pending', guardian_consent_status\) WHERE id = \?`).
		WithArgs(isMinor, isMinor, uid).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE profiles SET birth_year = \? WHERE user_id = \?`).WithArgs(birthYear, uid).WillReturnResult(sqlmock.NewResult(0, 0))
	next := "profile"
	if isMinor {
		next = "guardian"
	}
	usertest.ExpectAdvance(mock, uid, usertest.AdvanceRow{PrimaryRole: "player", Step: "verification", Verify: "verified", IsMinor: isMinor, Guardian: "not_required", Roles: []string{"player"}}, next)
	mock.ExpectCommit()
}

// expectFinishFailed expects finish() rejecting the identity with reason.
func expectFinishFailed(mock sqlmock.Sqlmock, attemptID int64, provider, reason string, owner int64, checkSubject bool) {
	mock.ExpectBegin()
	expectLockOpen(mock, attemptID, provider)
	expectUser(mock, false, "pending", false, "player", "Asha K")
	if checkSubject {
		expectSubject(mock, owner)
	}
	expectMarkFailed(mock, attemptID, reason)
	mock.ExpectCommit()
}

func expectStatus(mock sqlmock.Sqlmock, verify string, isMinor bool, provider, status string, last4, reason any) {
	expectUser(mock, false, verify, isMinor, "player", "Asha K")
	rows := sqlmock.NewRows([]string{"provider", "status", "aadhaar_last4", "failure_reason", "verified_at"})
	if provider != "" {
		var at any
		if status == "verified" {
			at = fixedNow
		}
		rows.AddRow(provider, status, last4, reason, at)
	}
	mock.ExpectQuery(`SELECT provider, status, aadhaar_last4, failure_reason, verified_at FROM identity_verifications WHERE user_id = \? ORDER BY status = 'verified' DESC, id DESC LIMIT 1`).
		WithArgs(uid).WillReturnRows(rows)
}

// ---- fake providers (loopback httptest servers) ----

type fakeDL struct {
	srv       *httptest.Server
	xml       string
	tokenCode int
	gotForm   url.Values
	gotAuth   string
	calls     int
}

func eAadhaar(dob, state, dist string) string {
	return fmt.Sprintf(`<?xml version="1.0"?><Certificate><CertificateData><KycRes><UidData uid="xxxxxxxx1234">`+
		`<Poi name="Asha Kulkarni" dob="%s" gender="F"/><Poa house="12" street="Main" state="%s" dist="%s" pc="613001"/>`+
		`<Pht>QUJDREVGRw==</Pht></UidData></KycRes></CertificateData></Certificate>`, dob, state, dist)
}

func newFakeDL(t *testing.T, d *app.Deps, xmlBody string) *fakeDL {
	t.Helper()
	f := &fakeDL{xml: xmlBody, tokenCode: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		f.calls++
		_ = r.ParseForm()
		f.gotForm = r.PostForm
		if f.tokenCode != http.StatusOK {
			w.WriteHeader(f.tokenCode)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","detail":"secret-ish"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "at-123", "digilockerid": "dl-subject-1", "name": "Asha Kulkarni", "dob": "15061995", "gender": "F"})
	})
	mux.HandleFunc("/api/3/xml/eaadhaar", func(w http.ResponseWriter, r *http.Request) {
		f.gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(f.xml))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	d.Secrets = secrets.Static{"athlix/test/digilocker": DigiLockerSecret{
		ClientID: "client-1", ClientSecret: "cs-1", RedirectURI: "http://test/v1/verification/digilocker/callback",
		AuthorizeURL: f.srv.URL + "/authorize", TokenURL: f.srv.URL + "/token", APIBaseURL: f.srv.URL + "/api",
	}}
	return f
}

type fakeAgg struct {
	srv      *httptest.Server
	identity map[string]any
	gotKey   string
	gotStart map[string]string
}

func newFakeAgg(t *testing.T, d *app.Deps) *fakeAgg {
	t.Helper()
	f := &fakeAgg{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/digilocker/sessions", func(w http.ResponseWriter, r *http.Request) {
		f.gotKey = r.Header.Get("X-Api-Key")
		_ = json.NewDecoder(r.Body).Decode(&f.gotStart)
		_ = json.NewEncoder(w).Encode(map[string]string{"sessionId": "sess-1", "authorizationUrl": "https://agg.example/s/sess-1"})
	})
	mux.HandleFunc("GET /v1/digilocker/sessions/sess-1/identity", func(w http.ResponseWriter, r *http.Request) {
		f.gotKey = r.Header.Get("X-Api-Key")
		_ = json.NewEncoder(w).Encode(f.identity)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	d.Secrets = secrets.Static{"athlix/test/kyc-aggregator": AggregatorSecret{Provider: "x", BaseURL: f.srv.URL, APIKey: "k-1", APISecret: "s-1"}}
	return f
}

func aggIdentity(dob, state, district string) map[string]any {
	return map[string]any{"status": "completed", "subjectId": "agg-subject-1", "maskedAadhaar": "XXXX-XXXX-5678", "name": "Asha Kulkarni",
		"dob": dob, "gender": "female", "address": map[string]string{"state": state, "district": district}}
}

// seedState stores a pending state as Start would.
func seedState(t *testing.T, e *env, state string, ps pendingState) {
	t.Helper()
	b, _ := json.Marshal(ps)
	if err := e.d.Redis.Set(context.Background(), stateKey(state), b, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
}

func callback(e *env, q string) *url.URL {
	w := testutil.Request(e.r, http.MethodGet, "/v1/verification/digilocker/callback?"+q, nil, nil)
	if w.Code != http.StatusFound {
		return nil
	}
	u, _ := url.Parse(w.Header().Get("Location"))
	return u
}

func assertDeepLink(t *testing.T, u *url.URL, status, reason string) {
	t.Helper()
	if u == nil {
		t.Fatal("expected a 302")
	}
	if u.Scheme != "athlix" || u.Host != "verification" || u.Path != "/result" {
		t.Fatalf("deep link = %s", u)
	}
	if got := u.Query().Get("status"); got != status {
		t.Fatalf("status = %q, want %q (%s)", got, status, u)
	}
	if got := u.Query().Get("reason"); got != reason {
		t.Fatalf("reason = %q, want %q", got, reason)
	}
}

// ---- start ----

func TestStartDirect(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderDirect, "verification.state_ttl_seconds": 300})
	f := newFakeDL(t, e.d, "")
	expectUser(e.mock, false, "unverified", false, "player", nil)
	expectOpenAttempt(e.mock, ProviderDirect, nil, 77)

	w := testutil.Request(e.r, http.MethodPost, "/v1/verification/digilocker/start", map[string]any{}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	var res StartResult
	_ = json.Unmarshal(decode(t, w).Data, &res)
	if !res.ExpiresAt.Equal(fixedNow.Add(300 * time.Second)) {
		t.Fatalf("expiresAt = %v", res.ExpiresAt)
	}
	au, err := url.Parse(res.AuthorizationURL)
	if err != nil || !strings.HasPrefix(res.AuthorizationURL, f.srv.URL+"/authorize?") {
		t.Fatalf("authorizationUrl = %q", res.AuthorizationURL)
	}
	q := au.Query()
	if q.Get("client_id") != "client-1" || q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" || q.Get("code_challenge") == "" {
		t.Fatalf("authorize query = %v", q)
	}
	if strings.Contains(res.AuthorizationURL, "cs-1") {
		t.Fatal("client secret leaked into the authorization URL")
	}
	// State is stored hashed, bound to the user, with the configured TTL.
	raw, err := e.d.Redis.Get(context.Background(), stateKey(q.Get("state"))).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var ps pendingState
	_ = json.Unmarshal(raw, &ps)
	if ps.UserID != uid || ps.AttemptID != 77 || ps.Provider != ProviderDirect || pkceChallenge(ps.Verifier) != q.Get("code_challenge") {
		t.Fatalf("pending state = %+v", ps)
	}
	if ttl := e.d.Redis.TTL(context.Background(), stateKey(q.Get("state"))).Val(); ttl <= 0 || ttl > 300*time.Second {
		t.Fatalf("ttl = %v", ttl)
	}
	if e.d.Redis.Exists(context.Background(), "dl:state:"+q.Get("state")).Val() != 0 {
		t.Fatal("raw state must not be a Redis key")
	}
}

func TestStartAggregatorStoresSessionRef(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderAggregator})
	f := newFakeAgg(t, e.d)
	expectUser(e.mock, false, "failed", false, "player", nil)
	expectOpenAttempt(e.mock, ProviderAggregator, "sess-1", 78)
	res, err := e.m.svc.Start(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if res.AuthorizationURL != "https://agg.example/s/sess-1" || f.gotKey != "k-1" {
		t.Fatalf("res %+v key %q", res, f.gotKey)
	}
	if !strings.HasPrefix(f.gotStart["redirectUrl"], "http://test/v1/verification/digilocker/callback?state=") || f.gotStart["reference"] == "" {
		t.Fatalf("start body = %v", f.gotStart)
	}
}

func TestStartRejections(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		setup    func(e *env)
		code     int
		errCode  string
	}{
		{"already verified", ProviderDirect, func(e *env) { expectUser(e.mock, false, "verified", false, "player", nil) }, http.StatusConflict, "INVALID_STATE"},
		{"no role yet", ProviderDirect, func(e *env) { expectUser(e.mock, false, "unverified", false, nil, nil) }, http.StatusConflict, "INVALID_STATE"},
		{"mock provider", ProviderMock, func(e *env) { expectUser(e.mock, false, "unverified", false, "player", nil) }, http.StatusConflict, "INVALID_STATE"},
		{"deleted user", ProviderDirect, func(e *env) { expectNoUser(e.mock) }, http.StatusNotFound, "NOT_FOUND"},
		{"secret missing", ProviderDirect, func(e *env) {
			e.d.Secrets = secrets.Static{}
			expectUser(e.mock, false, "unverified", false, "player", nil)
		}, http.StatusBadGateway, ""},
		{"daily limit", ProviderDirect, func(e *env) {
			newFakeDL(t, e.d, "")
			e.d.Redis.Set(context.Background(), "rl:verify:1", 2, time.Hour)
			expectUser(e.mock, false, "unverified", false, "player", nil)
		}, http.StatusTooManyRequests, "RATE_LIMITED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, map[string]any{"verification.provider": tc.provider, "verification.max_attempts_per_day": 2})
			tc.setup(e)
			w := testutil.Request(e.r, http.MethodPost, "/v1/verification/digilocker/start", map[string]any{}, nil)
			if w.Code != tc.code {
				t.Fatalf("code %d %s", w.Code, w.Body)
			}
			if tc.errCode != "" && decode(t, w).Error.Code != tc.errCode {
				t.Fatalf("error code %s", w.Body)
			}
		})
	}
}

// ---- callback: DigiLocker direct ----

func TestCallbackDirectAdultMatchesDistrictByAlias(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderDirect})
	f := newFakeDL(t, e.d, eAadhaar("15-06-1995", "Tamil Nadu", "Tanjore"))
	seedState(t, e, "st-1", pendingState{UserID: uid, AttemptID: 77, Provider: ProviderDirect, Verifier: "verifier-1"})
	expectVerified(e.mock, 77, ProviderDirect, "1234", "F", false, 1995)

	assertDeepLink(t, callback(e, "code=c-1&state=st-1"), "verified", "")
	if f.gotForm.Get("code") != "c-1" || f.gotForm.Get("code_verifier") != "verifier-1" || f.gotForm.Get("client_secret") != "cs-1" || f.gotForm.Get("grant_type") != "authorization_code" {
		t.Fatalf("token form = %v", f.gotForm)
	}
	if f.gotAuth != "Bearer at-123" {
		t.Fatalf("eaadhaar auth = %q", f.gotAuth)
	}
	got := e.published(t)
	if len(got) != 1 || got[0] != (events.VerificationPayload{UserID: uid, Status: "verified", NativeDistrictID: 26}) {
		t.Fatalf("events = %+v", got)
	}
	// The state is single-use: a replay does nothing (no SQL, no provider call, no event).
	assertDeepLink(t, callback(e, "code=c-1&state=st-1"), "failed", ReasonProviderError)
	if f.calls != 1 || len(e.published(t)) != 1 {
		t.Fatalf("replay reached the provider (%d calls) or published", f.calls)
	}
}

func TestCallbackDirectMinor(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderDirect})
	newFakeDL(t, e.d, eAadhaar("16-06-2010", "Kerala", "Ernakulam")) // 15 on fixedNow
	seedState(t, e, "st-2", pendingState{UserID: uid, AttemptID: 80, Provider: ProviderDirect, Verifier: "v"})
	expectVerified(e.mock, 80, ProviderDirect, "1234", "F", true, 2010)
	assertDeepLink(t, callback(e, "code=c&state=st-2"), "minor", "")
	got := e.published(t)
	if len(got) != 1 || !got[0].IsMinor || got[0].NativeDistrictID != 9001 {
		t.Fatalf("events = %+v", got)
	}
}

func TestCallbackDirectTooYoung(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderDirect, "auth.min_age_years": 10})
	newFakeDL(t, e.d, eAadhaar("16-06-2016", "Tamil Nadu", "Thanjavur")) // 9
	seedState(t, e, "st-3", pendingState{UserID: uid, AttemptID: 81, Provider: ProviderDirect, Verifier: "v"})
	expectFinishFailed(e.mock, 81, ProviderDirect, ReasonTooYoung, 0, false)
	assertDeepLink(t, callback(e, "code=c&state=st-3"), "failed", ReasonTooYoung)
	got := e.published(t)
	if len(got) != 1 || got[0] != (events.VerificationPayload{UserID: uid, Status: "failed"}) {
		t.Fatalf("failed attempts publish without a district: %+v", got)
	}
}

func TestCallbackMinAgeIsConfigDriven(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderDirect, "auth.min_age_years": 8, "guardian.minor_age_years": 16})
	newFakeDL(t, e.d, eAadhaar("16-06-2016", "", "")) // 9: allowed and a minor
	seedState(t, e, "st-4", pendingState{UserID: uid, AttemptID: 82, Provider: ProviderDirect, Verifier: "v"})
	expectVerified(e.mock, 82, ProviderDirect, "1234", "F", true, 2016)
	assertDeepLink(t, callback(e, "code=c&state=st-4"), "minor", "")

	// guardian.minor_age_years = 16: a 17-year-old is an adult.
	e2 := newEnv(t, map[string]any{"verification.provider": ProviderDirect, "guardian.minor_age_years": 16})
	newFakeDL(t, e2.d, eAadhaar("01-01-2009", "", ""))
	seedState(t, e2, "st-5", pendingState{UserID: uid, AttemptID: 83, Provider: ProviderDirect, Verifier: "v"})
	expectVerified(e2.mock, 83, ProviderDirect, "1234", "F", false, 2009)
	assertDeepLink(t, callback(e2, "code=c&state=st-5"), "verified", "")
}

func TestCallbackAlreadyLinked(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderDirect})
	newFakeDL(t, e.d, eAadhaar("15-06-1995", "Tamil Nadu", "Tanjore"))
	seedState(t, e, "st-6", pendingState{UserID: uid, AttemptID: 84, Provider: ProviderDirect, Verifier: "v"})
	expectFinishFailed(e.mock, 84, ProviderDirect, ReasonAlreadyLinked, 2, true)
	assertDeepLink(t, callback(e, "code=c&state=st-6"), "failed", ReasonAlreadyLinked)
}

func TestCallbackDuplicateKeyRaceIsAlreadyLinked(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderDirect})
	newFakeDL(t, e.d, eAadhaar("15-06-1995", "", ""))
	seedState(t, e, "st-7", pendingState{UserID: uid, AttemptID: 85, Provider: ProviderDirect, Verifier: "v"})
	e.mock.ExpectBegin()
	expectLockOpen(e.mock, 85, ProviderDirect)
	expectUser(e.mock, false, "pending", false, "player", nil)
	expectSubject(e.mock, 0)
	expectMarkVerified(e.mock, 85, "1234", "F").WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"})
	expectMarkFailed(e.mock, 85, ReasonAlreadyLinked)
	e.mock.ExpectCommit()
	assertDeepLink(t, callback(e, "code=c&state=st-7"), "failed", ReasonAlreadyLinked)
}

func TestCallbackNameMatchRule(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderDirect, "verification.name_match_required": true})
	newFakeDL(t, e.d, eAadhaar("15-06-1995", "", ""))
	seedState(t, e, "st-8", pendingState{UserID: uid, AttemptID: 86, Provider: ProviderDirect, Verifier: "v"})
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(`SELECT status, provider FROM identity_verifications WHERE id = \? AND user_id = \? FOR UPDATE`).
		WithArgs(int64(86), uid).WillReturnRows(sqlmock.NewRows([]string{"status", "provider"}).AddRow("initiated", ProviderDirect))
	expectUser(e.mock, true, "pending", false, "player", "Ravi Kumar")
	expectUser(e.mock, false, "pending", false, "player", "Ravi Kumar")
	expectMarkFailed(e.mock, 86, ReasonNameMismatch)
	e.mock.ExpectCommit()
	assertDeepLink(t, callback(e, "code=c&state=st-8"), "failed", ReasonNameMismatch)

	// "Asha K" resembles "Asha Kulkarni" and passes.
	e2 := newEnv(t, map[string]any{"verification.provider": ProviderDirect, "verification.name_match_required": true})
	newFakeDL(t, e2.d, eAadhaar("15-06-1995", "", ""))
	seedState(t, e2, "st-9", pendingState{UserID: uid, AttemptID: 87, Provider: ProviderDirect, Verifier: "v"})
	expectVerified(e2.mock, 87, ProviderDirect, "1234", "F", false, 1995)
	assertDeepLink(t, callback(e2, "code=c&state=st-9"), "verified", "")
}

func TestCallbackCancelledAndProviderErrors(t *testing.T) {
	t.Run("user cancelled", func(t *testing.T) {
		e := newEnv(t, map[string]any{"verification.provider": ProviderDirect})
		f := newFakeDL(t, e.d, "")
		seedState(t, e, "s", pendingState{UserID: uid, AttemptID: 90, Provider: ProviderDirect, Verifier: "v"})
		e.mock.ExpectBegin()
		expectLockOpen(e.mock, 90, ProviderDirect)
		expectMarkFailed(e.mock, 90, ReasonCancelled)
		e.mock.ExpectCommit()
		assertDeepLink(t, callback(e, "error=access_denied&state=s"), "failed", ReasonCancelled)
		if f.calls != 0 {
			t.Fatal("provider must not be called after a cancel")
		}
	})
	t.Run("token exchange fails", func(t *testing.T) {
		e := newEnv(t, map[string]any{"verification.provider": ProviderDirect})
		f := newFakeDL(t, e.d, "")
		f.tokenCode = http.StatusBadRequest
		seedState(t, e, "s", pendingState{UserID: uid, AttemptID: 91, Provider: ProviderDirect, Verifier: "v"})
		e.mock.ExpectBegin()
		expectLockOpen(e.mock, 91, ProviderDirect)
		expectMarkFailed(e.mock, 91, ReasonProviderError)
		e.mock.ExpectCommit()
		assertDeepLink(t, callback(e, "code=c&state=s"), "failed", ReasonProviderError)
		if len(e.published(t)) != 1 {
			t.Fatal("failed attempt must publish verification.completed")
		}
	})
	t.Run("bad xml", func(t *testing.T) {
		e := newEnv(t, map[string]any{"verification.provider": ProviderDirect})
		newFakeDL(t, e.d, "<html>oops")
		seedState(t, e, "s", pendingState{UserID: uid, AttemptID: 92, Provider: ProviderDirect, Verifier: "v"})
		e.mock.ExpectBegin()
		expectLockOpen(e.mock, 92, ProviderDirect)
		expectMarkFailed(e.mock, 92, ReasonProviderError)
		e.mock.ExpectCommit()
		assertDeepLink(t, callback(e, "code=c&state=s"), "failed", ReasonProviderError)
	})
	t.Run("unknown or missing state", func(t *testing.T) {
		e := newEnv(t, map[string]any{"verification.provider": ProviderDirect})
		assertDeepLink(t, callback(e, "code=c&state=never-issued"), "failed", ReasonProviderError)
		assertDeepLink(t, callback(e, "code=c"), "failed", ReasonProviderError)
		assertDeepLink(t, callback(e, "code=c&state="+strings.Repeat("a", 300)), "failed", ReasonProviderError)
	})
}

// The state carries the user; an attempt that is not that user's (or no longer open) is never
// touched: the scoped SELECT finds nothing and no write follows.
func TestCallbackAttemptScopedToStateOwner(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderDirect})
	newFakeDL(t, e.d, eAadhaar("15-06-1995", "Tamil Nadu", "Tanjore"))
	seedState(t, e, "s", pendingState{UserID: uid, AttemptID: 999, Provider: ProviderDirect, Verifier: "v"})
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(`SELECT status, provider FROM identity_verifications WHERE id = \? AND user_id = \? FOR UPDATE`).
		WithArgs(int64(999), uid).WillReturnRows(sqlmock.NewRows([]string{"status", "provider"}))
	e.mock.ExpectRollback()
	assertDeepLink(t, callback(e, "code=c&state=s"), "failed", ReasonProviderError)
	if len(e.published(t)) != 0 {
		t.Fatal("no event for a stale attempt")
	}
}

// ---- callback: aggregator ----

func TestCallbackAggregatorNoDistrictMatch(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderAggregator})
	f := newFakeAgg(t, e.d)
	f.identity = aggIdentity("1995-06-15", "Tamil Nadu", "Atlantis")
	seedState(t, e, "s", pendingState{UserID: uid, AttemptID: 95, Provider: ProviderAggregator, Ref: "sess-1"})
	expectVerified(e.mock, 95, ProviderAggregator, "5678", "F", false, 1995)
	assertDeepLink(t, callback(e, "state=s"), "verified", "")
	got := e.published(t)
	if len(got) != 1 || got[0].NativeDistrictID != 0 || got[0].Status != "verified" {
		t.Fatalf("events = %+v", got)
	}
	if f.gotKey != "k-1" {
		t.Fatalf("api key header = %q", f.gotKey)
	}
}

func TestCallbackAggregatorAliasAndCancel(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.provider": ProviderAggregator})
	f := newFakeAgg(t, e.d)
	f.identity = aggIdentity("15-06-1995", "tamil nadu", "Kovai")
	seedState(t, e, "s1", pendingState{UserID: uid, AttemptID: 96, Provider: ProviderAggregator, Ref: "sess-1"})
	expectVerified(e.mock, 96, ProviderAggregator, "5678", "F", false, 1995)
	assertDeepLink(t, callback(e, "state=s1"), "verified", "")
	if got := e.published(t); len(got) != 1 || got[0].NativeDistrictID != 4 {
		t.Fatalf("events = %+v", got)
	}

	f.identity = map[string]any{"status": "cancelled"}
	seedState(t, e, "s2", pendingState{UserID: uid, AttemptID: 97, Provider: ProviderAggregator, Ref: "sess-1"})
	e.mock.ExpectBegin()
	expectLockOpen(e.mock, 97, ProviderAggregator)
	expectMarkFailed(e.mock, 97, ReasonCancelled)
	e.mock.ExpectCommit()
	assertDeepLink(t, callback(e, "state=s2"), "failed", ReasonCancelled)
}

// ---- mock ----

func mockReq(e *env, b any) *httptest.ResponseRecorder {
	return testutil.Request(e.r, http.MethodPost, "/v1/verification/mock/complete", b, nil)
}

func TestMockAdultPassesDistrictThrough(t *testing.T) {
	e := newEnv(t, nil) // provider defaults to mock
	expectUser(e.mock, false, "unverified", false, "player", "Asha K")
	expectOpenAttempt(e.mock, ProviderMock, nil, 70)
	expectVerified(e.mock, 70, ProviderMock, "0000", "U", false, fixedNow.Year()-25)
	expectStatus(e.mock, "verified", false, ProviderMock, "verified", "0000", nil)
	w := mockReq(e, map[string]any{"outcome": "adult", "nativeDistrictId": 26})
	if w.Code != http.StatusOK {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	var st Status
	_ = json.Unmarshal(decode(t, w).Data, &st)
	if st.Status != "verified" || st.IsMinor || *st.Provider != ProviderMock || *st.AadhaarLast4 != "0000" || st.AttemptsLeftToday != 2 || st.FailureReason != nil {
		t.Fatalf("status = %+v", st)
	}
	if got := e.published(t); len(got) != 1 || got[0] != (events.VerificationPayload{UserID: uid, Status: "verified", NativeDistrictID: 26}) {
		t.Fatalf("events = %+v", got)
	}
}

func TestMockOutcomes(t *testing.T) {
	t.Run("minor without district", func(t *testing.T) {
		e := newEnv(t, nil)
		expectUser(e.mock, false, "unverified", false, "player", nil)
		expectOpenAttempt(e.mock, ProviderMock, nil, 71)
		expectVerified(e.mock, 71, ProviderMock, "0000", "U", true, fixedNow.Year()-17)
		expectStatus(e.mock, "verified", true, ProviderMock, "verified", "0000", nil)
		if w := mockReq(e, map[string]any{"outcome": "minor"}); w.Code != http.StatusOK {
			t.Fatalf("code %d %s", w.Code, w.Body)
		}
		if got := e.published(t); len(got) != 1 || !got[0].IsMinor || got[0].NativeDistrictID != 0 {
			t.Fatalf("events = %+v", got)
		}
	})
	for _, tc := range []struct{ outcome, reason string }{{"too_young", ReasonTooYoung}, {"duplicate", ReasonAlreadyLinked}, {"failed", ReasonProviderError}} {
		t.Run(tc.outcome, func(t *testing.T) {
			e := newEnv(t, nil)
			expectUser(e.mock, false, "unverified", false, "player", nil)
			expectOpenAttempt(e.mock, ProviderMock, nil, 72)
			if tc.outcome == "too_young" {
				expectFinishFailed(e.mock, 72, ProviderMock, tc.reason, 0, false)
			} else {
				e.mock.ExpectBegin()
				expectLockOpen(e.mock, 72, ProviderMock)
				expectMarkFailed(e.mock, 72, tc.reason)
				e.mock.ExpectCommit()
			}
			expectStatus(e.mock, "failed", false, ProviderMock, "failed", nil, tc.reason)
			w := mockReq(e, map[string]any{"outcome": tc.outcome, "nativeDistrictId": 26})
			var st Status
			_ = json.Unmarshal(decode(t, w).Data, &st)
			if w.Code != http.StatusOK || st.Status != "failed" || st.FailureReason == nil || *st.FailureReason != tc.reason {
				t.Fatalf("code %d status %+v", w.Code, st)
			}
			if got := e.published(t); len(got) != 1 || got[0].Status != "failed" || got[0].NativeDistrictID != 0 {
				t.Fatalf("events = %+v", got)
			}
		})
	}
}

func TestMockValidationAndAvailability(t *testing.T) {
	e := newEnv(t, nil)
	w := mockReq(e, map[string]any{"outcome": "adult", "nativeDistrictId": 424242})
	if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["nativeDistrictId"] == "" {
		t.Fatalf("unknown district: %d %s", w.Code, w.Body)
	}
	w = mockReq(e, map[string]any{"outcome": "superhero"})
	if w.Code != http.StatusUnprocessableEntity || decode(t, w).Error.Fields["outcome"] == "" {
		t.Fatalf("bad outcome: %d %s", w.Code, w.Body)
	}

	other := newEnv(t, map[string]any{"verification.provider": ProviderDirect})
	if w := mockReq(other, map[string]any{"outcome": "adult"}); w.Code != http.StatusNotFound {
		t.Fatalf("provider not mock: %d", w.Code)
	}
	prod := newEnv(t, nil)
	prod.d.Cfg.Env = config.EnvProd
	if w := mockReq(prod, map[string]any{"outcome": "adult"}); w.Code != http.StatusNotFound {
		t.Fatalf("prod: %d", w.Code)
	}
	limited := newEnv(t, map[string]any{"verification.max_attempts_per_day": 1})
	limited.d.Redis.Set(context.Background(), "rl:verify:1", 1, time.Hour)
	expectUser(limited.mock, false, "unverified", false, "player", nil)
	if w := mockReq(limited, map[string]any{"outcome": "adult"}); w.Code != http.StatusTooManyRequests {
		t.Fatalf("limit: %d", w.Code)
	}
}

// ---- status ----

func TestStatus(t *testing.T) {
	e := newEnv(t, map[string]any{"verification.max_attempts_per_day": 5})
	e.d.Redis.Set(context.Background(), "rl:verify:1", 2, time.Hour)
	expectStatus(e.mock, "failed", false, ProviderDirect, "failed", nil, ReasonCancelled)
	w := testutil.Request(e.r, http.MethodGet, "/v1/verification/status", nil, nil)
	var st Status
	_ = json.Unmarshal(decode(t, w).Data, &st)
	if w.Code != http.StatusOK || st.Status != "failed" || *st.FailureReason != ReasonCancelled || st.AttemptsLeftToday != 3 || st.AadhaarLast4 != nil || st.VerifiedAt != nil {
		t.Fatalf("status %d %+v", w.Code, st)
	}

	e2 := newEnv(t, nil)
	expectStatus(e2.mock, "unverified", false, "", "", nil, nil)
	w = testutil.Request(e2.r, http.MethodGet, "/v1/verification/status", nil, nil)
	if !strings.Contains(w.Body.String(), `"provider":null`) || !strings.Contains(w.Body.String(), `"attemptsLeftToday":3`) {
		t.Fatalf("fresh status = %s", w.Body)
	}

	e3 := newEnv(t, nil)
	expectStatus(e3.mock, "verified", false, ProviderDirect, "verified", "1234", nil)
	w = testutil.Request(e3.r, http.MethodGet, "/v1/verification/status", nil, nil)
	_ = json.Unmarshal(decode(t, w).Data, &st)
	if st.VerifiedAt == nil || !st.VerifiedAt.Equal(fixedNow) || *st.AadhaarLast4 != "1234" {
		t.Fatalf("verified status %+v", st)
	}
}

// ---- jobs and events ----

func TestMinorRecheck(t *testing.T) {
	e := newEnv(t, map[string]any{"guardian.minor_age_years": 18})
	turned, _ := e.d.Crypto.EncryptString("2008-06-14") // 18 yesterday
	still, _ := e.d.Crypto.EncryptString("2008-06-16")  // 18 tomorrow
	e.mock.ExpectQuery(`SELECT u.id, iv.dob_enc FROM users u JOIN identity_verifications iv`).WithArgs(int64(0), 500).
		WillReturnRows(sqlmock.NewRows([]string{"id", "dob_enc"}).AddRow(5, turned).AddRow(6, still).AddRow(7, []byte("garbage")))
	e.mock.ExpectBegin()
	e.mock.ExpectExec(`UPDATE users SET is_minor = 0, guardian_consent_status = IF\(guardian_consent_status = 'revoked', guardian_consent_status, 'not_required'\) WHERE id = \? AND is_minor = 1`).WithArgs(int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	usertest.ExpectAdvance(e.mock, 5, usertest.AdvanceRow{PrimaryRole: "player", Step: "guardian", Verify: "verified", Guardian: "pending", Roles: []string{"player"}}, "profile")
	e.mock.ExpectCommit()
	if err := e.m.Jobs()["verification.minor_recheck"](context.Background()); err != nil {
		t.Fatal(err)
	}

	// A user already cleared (concurrent run) is skipped without Advance.
	e2 := newEnv(t, nil)
	turned2, _ := e2.d.Crypto.EncryptString("2008-06-14")
	e2.mock.ExpectQuery(`SELECT u.id, iv.dob_enc FROM users u`).WillReturnRows(sqlmock.NewRows([]string{"id", "dob_enc"}).AddRow(5, turned2))
	e2.mock.ExpectBegin()
	e2.mock.ExpectExec(`UPDATE users SET is_minor = 0`).WithArgs(int64(5)).WillReturnResult(sqlmock.NewResult(0, 0))
	e2.mock.ExpectCommit()
	if err := e2.m.svc.MinorRecheck(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestUserDeletedWipeIsIdempotent(t *testing.T) {
	e := newEnv(t, nil)
	h := e.m.EventHandlers()[events.UserDeleted]
	wipe := `UPDATE identity_verifications SET name_enc = NULL, dob_enc = NULL, gender = NULL, aadhaar_last4 = NULL`
	e.mock.ExpectExec(wipe).WithArgs(uid).WillReturnResult(sqlmock.NewResult(0, 2))
	e.mock.ExpectExec(wipe).WithArgs(uid).WillReturnResult(sqlmock.NewResult(0, 0))
	ev := events.New(events.UserDeleted, uid, events.UserPayload{UserID: uid})
	if err := h(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if err := h(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if err := h(context.Background(), events.New(events.UserDeleted, 0, events.UserPayload{})); err != nil {
		t.Fatal("malformed payload must be dropped, not retried")
	}
	e.mock.ExpectExec(wipe).WithArgs(uid).WillReturnError(errors.New("db down"))
	if err := h(context.Background(), ev); err == nil {
		t.Fatal("db error must be returned for retry")
	}
}

func TestBirthYear(t *testing.T) {
	e := newEnv(t, nil)
	enc, _ := e.d.Crypto.EncryptString("2010-03-04")
	q := `SELECT dob_enc FROM identity_verifications WHERE user_id = \? AND status = 'verified'`
	e.mock.ExpectQuery(q).WithArgs(uid).WillReturnRows(sqlmock.NewRows([]string{"dob_enc"}).AddRow(enc))
	if y, ok, err := BirthYear(context.Background(), e.d, nil, uid); err != nil || !ok || y != 2010 {
		t.Fatalf("got %d %v %v", y, ok, err)
	}
	e.mock.ExpectQuery(q).WithArgs(uid).WillReturnRows(sqlmock.NewRows([]string{"dob_enc"}))
	if _, ok, err := BirthYear(context.Background(), e.d, nil, uid); err != nil || ok {
		t.Fatalf("unverified: %v %v", ok, err)
	}
}

// ---- pure helpers ----

func TestAgeOnUsesIST(t *testing.T) {
	dob := time.Date(2008, 6, 15, 0, 0, 0, 0, time.UTC)
	// 2026-06-14 20:00 UTC is already 15 June in IST.
	if a := ageOn(dob, time.Date(2026, 6, 14, 20, 0, 0, 0, time.UTC)); a != 18 {
		t.Fatalf("age = %d", a)
	}
	if a := ageOn(dob, time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)); a != 17 {
		t.Fatalf("age = %d", a)
	}
}

func TestNameResembles(t *testing.T) {
	for _, tc := range []struct {
		display, verified string
		want              bool
	}{
		{"Asha K", "Asha Kulkarni", true},
		{"asha kulkarni", "ASHA KULKARNI", true},
		{"A. Kulkarni", "Asha Kulkarni", true},
		{"Ravi", "Asha Kulkarni", false},
		{"", "Asha", true},
	} {
		if got := nameResembles(tc.display, tc.verified); got != tc.want {
			t.Errorf("%q vs %q = %v", tc.display, tc.verified, got)
		}
	}
}

func TestParseEAadhaarKeepsOnlyNeededFields(t *testing.T) {
	id, err := parseEAadhaar(strings.NewReader(eAadhaar("01-02-2000", "Tamil Nadu", "Tanjore")))
	if err != nil {
		t.Fatal(err)
	}
	if id.AadhaarLast4 != "1234" || id.Name != "Asha Kulkarni" || id.Gender != "F" || id.DOB.Format(dobLayout) != "2000-02-01" || id.StateName != "Tamil Nadu" || id.DistrictName != "Tanjore" {
		t.Fatalf("identity = %+v", id)
	}
	if _, err := parseEAadhaar(strings.NewReader("<x/>")); err == nil {
		t.Fatal("xml without UidData must fail")
	}
	if _, err := parseDOB("31/12/1999"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseDOB("yesterday"); err == nil {
		t.Fatal("bad dob accepted")
	}
	if normGender("Female") != "F" || normGender("x") != "U" || normGender("transgender") != "T" {
		t.Fatal("normGender")
	}
}

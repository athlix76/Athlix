package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/config"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/testutil"
)

var fixedNow = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

// ---- fakes ----

type fakePlay struct {
	sub   PlaySubscription
	err   error
	pkg   string
	calls int
	acks  int
}

func (f *fakePlay) PackageName(context.Context) (string, error) { return f.pkg, nil }
func (f *fakePlay) Subscription(context.Context, string) (PlaySubscription, error) {
	f.calls++
	return f.sub, f.err
}
func (f *fakePlay) Acknowledge(context.Context, string, string) error { f.acks++; return nil }

type fakePush struct{ err error }

func (f *fakePush) Verify(context.Context, string) error { return f.err }

type fakeApple struct {
	subs    []AppleSubscription
	err     error
	note    AppleNotification
	noteErr error
	calls   int
}

func (f *fakeApple) Statuses(context.Context, string) ([]AppleSubscription, error) {
	f.calls++
	return f.subs, f.err
}
func (f *fakeApple) VerifyNotification(context.Context, string) (AppleNotification, error) {
	return f.note, f.noteErr
}

type env struct {
	m      *Module
	d      *app.Deps
	mock   sqlmock.Sqlmock
	play   *fakePlay
	push   *fakePush
	apple  *fakeApple
	events []events.Event
}

// setup builds the module with fake stores, a fixed clock and an event recorder.
func setup(t *testing.T, cfg map[string]any) *env {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return fixedNow }
	e := &env{d: d, mock: mock, play: &fakePlay{pkg: "app.athlix"}, push: &fakePush{}, apple: &fakeApple{}}
	for _, typ := range []string{events.SubscriptionActivated, events.SubscriptionExpired} {
		d.Dispatcher.On(typ, func(_ context.Context, ev events.Event) error { e.events = append(e.events, ev); return nil })
	}
	e.m = New(d)
	e.m.svc.play, e.m.svc.push, e.m.svc.apple = e.play, e.push, e.apple
	return e
}

func (e *env) user(id int64) *gin.Engine {
	r, routes := testutil.UserEngine(testutil.VerifiedUser(id))
	e.m.Routes(routes)
	return r
}

func (e *env) admin(perms ...string) *gin.Engine {
	r, routes := testutil.AdminEngine(9, perms)
	e.m.Routes(routes)
	return r
}

func (e *env) eventTypes() []string {
	out := []string{}
	for _, ev := range e.events {
		out = append(out, ev.Type)
	}
	return out
}

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Meta  map[string]any  `json:"meta"`
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func parse(t *testing.T, w *httptest.ResponseRecorder, wantStatus int) envelope {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status %d want %d body %s", w.Code, wantStatus, w.Body.String())
	}
	var env envelope
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatalf("bad json %s", w.Body.String())
		}
	}
	return env
}

func q(s string) string { return regexp.QuoteMeta(s) }

// ---- row builders ----

func planRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "code", "audience", "name", "description", "price_paise", "billing_period",
		"store_product_id_android", "store_product_id_ios", "active", "sort_order"})
}

func addPlan(r *sqlmock.Rows, id int64, active bool) *sqlmock.Rows {
	return r.AddRow(id, "player_plus_monthly", "player", "Player Plus", "Stand out.", 9900, "monthly", "plus_android", "plus_ios", active, 1)
}

func benefitRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"plan_id", "benefit_key", "value", "label", "sort_order"})
}

func subRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "user_id", "plan_id", "store", "store_transaction_id", "status", "auto_renew", "started_at", "expires_at"})
}

func (e *env) expectRender(planID int64) {
	e.mock.ExpectQuery(q("FROM premium_plans WHERE id = ?")).WithArgs(planID).WillReturnRows(addPlan(planRows(), planID, true))
	e.mock.ExpectQuery(q("FROM premium_plan_benefits")).WithArgs(planID).
		WillReturnRows(benefitRows().AddRow(planID, "profile_boost", []byte("true"), "Boosted profile", 1))
}

func dupErr() error { return &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"} }

// ---- storeAccountToken ----

func TestStoreAccountTokenIsStableUUIDPerUser(t *testing.T) {
	d, _ := testutil.Deps(t, nil)
	a, b := storeAccountToken(d.Crypto, 42), storeAccountToken(d.Crypto, 42)
	if a != b || len(a) != 36 || strings.Count(a, "-") != 4 {
		t.Fatalf("token %q %q", a, b)
	}
	if storeAccountToken(d.Crypto, 43) == a {
		t.Fatal("tokens must differ per user")
	}
	h := d.Crypto.Hash("store_account", "42")
	want := append([]byte{}, h[:16]...)
	want[6] = want[6]&0x0f | 0x40
	want[8] = want[8]&0x3f | 0x80
	if strings.ReplaceAll(a, "-", "") != hexs(want) || a[14] != '4' || !strings.ContainsRune("89ab", rune(a[19])) {
		t.Fatalf("token %s must be the first 16 hash bytes with v4/variant bits", a)
	}
}

func hexs(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&15])
	}
	return string(out)
}

// ---- plans / me ----

func TestPlansEmptyWhenPremiumDisabled(t *testing.T) {
	e := setup(t, map[string]any{"premium.enabled": false})
	env := parse(t, testutil.Request(e.user(42), "GET", "/v1/premium/plans", nil, nil), 200)
	if string(env.Data) != "[]" {
		t.Fatalf("data %s", env.Data)
	}
}

func TestPlansListedWhenEnabled(t *testing.T) {
	e := setup(t, map[string]any{"premium.enabled": true})
	e.mock.ExpectQuery(q("FROM premium_plans WHERE 1=1 AND active = 1 AND audience = ? ORDER BY sort_order, id")).WithArgs("player").
		WillReturnRows(planRows().AddRow(1, "player_plus_monthly", "player", "Player Plus", nil, 9900, "monthly", nil, nil, true, 1))
	e.mock.ExpectQuery(q("FROM premium_plan_benefits WHERE plan_id IN (?)")).WithArgs(int64(1)).
		WillReturnRows(benefitRows().AddRow(1, "extra_communities", []byte("25"), "Up to 25", 1))
	env := parse(t, testutil.Request(e.user(42), "GET", "/v1/premium/plans?audience=player", nil, nil), 200)
	var plans []map[string]any
	_ = json.Unmarshal(env.Data, &plans)
	if len(plans) != 1 || plans[0]["code"] != "player_plus_monthly" || plans[0]["pricePaise"].(float64) != 9900 {
		t.Fatalf("plans %s", env.Data)
	}
	if v, ok := plans[0]["storeProductIdAndroid"]; !ok || v != nil {
		t.Fatal("optional fields must be present as null")
	}
	if _, ok := plans[0]["description"]; !ok {
		t.Fatal("description must be present")
	}
	b := plans[0]["benefits"].([]any)[0].(map[string]any)
	if b["key"] != "extra_communities" || b["value"].(float64) != 25 {
		t.Fatalf("benefit %v", b)
	}
}

func TestPlansRejectsUnknownAudience(t *testing.T) {
	e := setup(t, map[string]any{"premium.enabled": true})
	env := parse(t, testutil.Request(e.user(42), "GET", "/v1/premium/plans?audience=fan", nil, nil), 422)
	if env.Error.Fields["audience"] == "" {
		t.Fatal("expected audience field error")
	}
}

func TestMySubscriptionActiveAndBenefits(t *testing.T) {
	e := setup(t, nil)
	e.mock.ExpectQuery(q("WHERE user_id = ? AND status IN ('active','grace') AND expires_at > ?")).WithArgs(int64(42), fixedNow).
		WillReturnRows(subRows().AddRow(4, 42, 1, "google_play", "abc", "active", true, fixedNow.Add(-time.Hour), fixedNow.Add(720*time.Hour)))
	e.expectRender(1)
	e.mock.ExpectQuery(q("SELECT b.benefit_key, b.value")).WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"benefit_key", "value"}).AddRow("profile_boost", []byte("true")))
	env := parse(t, testutil.Request(e.user(42), "GET", "/v1/me/subscription", nil, nil), 200)
	var out struct {
		Active *struct {
			ID        int64  `json:"id"`
			Store     string `json:"store"`
			Status    string `json:"status"`
			AutoRenew bool   `json:"autoRenew"`
			ExpiresAt string `json:"expiresAt"`
			Plan      struct {
				ID int64 `json:"id"`
			} `json:"plan"`
		} `json:"active"`
		Benefits map[string]any `json:"benefits"`
	}
	_ = json.Unmarshal(env.Data, &out)
	if out.Active == nil || out.Active.ID != 4 || out.Active.Plan.ID != 1 || out.Active.Store != "google_play" || !out.Active.AutoRenew {
		t.Fatalf("active %s", env.Data)
	}
	if out.Active.ExpiresAt != "2026-12-13T10:00:00Z" {
		t.Fatalf("expiresAt %s", out.Active.ExpiresAt)
	}
	if out.Benefits["profile_boost"] != true {
		t.Fatalf("benefits %v", out.Benefits)
	}
}

func TestMySubscriptionNone(t *testing.T) {
	e := setup(t, nil)
	e.mock.ExpectQuery(q("FROM subscriptions")).WillReturnRows(subRows())
	e.mock.ExpectQuery(q("SELECT b.benefit_key, b.value")).WillReturnRows(sqlmock.NewRows([]string{"benefit_key", "value"}))
	env := parse(t, testutil.Request(e.user(42), "GET", "/v1/me/subscription", nil, nil), 200)
	if string(env.Data) != `{"active":null,"benefits":{}}` {
		t.Fatalf("data %s", env.Data)
	}
}

// ---- verify purchases ----

func TestVerifyValidation(t *testing.T) {
	e := setup(t, nil)
	r := e.user(42)
	env := parse(t, testutil.Request(r, "POST", "/v1/premium/purchases/verify", map[string]any{"store": "paypal"}, nil), 422)
	if env.Error.Fields["store"] == "" || env.Error.Fields["productId"] == "" {
		t.Fatalf("fields %v", env.Error.Fields)
	}
	env = parse(t, testutil.Request(r, "POST", "/v1/premium/purchases/verify", map[string]any{"store": "google_play", "productId": "p"}, nil), 422)
	if env.Error.Fields["purchaseToken"] == "" {
		t.Fatalf("fields %v", env.Error.Fields)
	}
	env = parse(t, testutil.Request(r, "POST", "/v1/premium/purchases/verify", map[string]any{"store": "app_store", "productId": "p", "transactionId": "../x"}, nil), 422)
	if env.Error.Fields["transactionId"] == "" {
		t.Fatalf("fields %v", env.Error.Fields)
	}
}

func (e *env) activePlay(userID int64) PlaySubscription {
	return PlaySubscription{ProductID: "plus_android", State: "SUBSCRIPTION_STATE_ACTIVE",
		ObfuscatedAccountID: storeAccountToken(e.d.Crypto, userID), StartedAt: fixedNow.Add(-time.Minute),
		ExpiresAt: fixedNow.Add(30 * 24 * time.Hour), AutoRenew: true}
}

func TestVerifyPlayHappyPathStoresOnlyTokenHash(t *testing.T) {
	e := setup(t, map[string]any{"premium.enabled": false}) // purchases are honoured even when disabled
	e.play.sub = e.activePlay(42)
	h := crypto.SHA256("tok-1")
	e.mock.ExpectQuery(q("WHERE store_product_id_android = ?")).WithArgs("plus_android").WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("WHERE store = ? AND store_transaction_id = ? FOR UPDATE")).WithArgs("google_play", hexs(h)).WillReturnRows(subRows())
	e.mock.ExpectExec(q("INSERT INTO subscriptions")).
		WithArgs(int64(42), int64(1), "google_play", hexs(h), h, "active", true, sqlmock.AnyArg(), e.play.sub.ExpiresAt).
		WillReturnResult(sqlmock.NewResult(7, 1))
	e.mock.ExpectCommit()
	e.expectRender(1)
	env := parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/verify",
		map[string]any{"store": "google_play", "productId": "plus_android", "purchaseToken": "tok-1"}, nil), 200)
	var sub Subscription
	_ = json.Unmarshal(env.Data, &sub)
	if sub.ID != 7 || sub.Status != "active" || sub.Plan.ID != 1 {
		t.Fatalf("sub %s", env.Data)
	}
	if got := e.eventTypes(); len(got) != 1 || got[0] != events.SubscriptionActivated {
		t.Fatalf("events %v", got)
	}
	var p events.SubscriptionPayload
	_ = e.events[0].Decode(&p)
	if p.SubscriptionID != 7 || p.UserID != 42 || p.PlanID != 1 {
		t.Fatalf("payload %+v", p)
	}
	if e.play.acks != 1 {
		t.Fatal("purchase must be acknowledged")
	}
}

func TestVerifyPlayAccountTokenMismatchIs404(t *testing.T) {
	e := setup(t, nil)
	e.play.sub = e.activePlay(99) // bought by another account
	env := parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/verify",
		map[string]any{"store": "google_play", "productId": "plus_android", "purchaseToken": "tok-1"}, nil), 404)
	if env.Error.Code != "NOT_FOUND" {
		t.Fatal(env.Error.Code)
	}
	e.play.sub = e.activePlay(42)
	e.play.sub.ProductID = "other"
	parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/verify",
		map[string]any{"store": "google_play", "productId": "plus_android", "purchaseToken": "tok-1"}, nil), 404)
}

func TestVerifyPlayRowOfAnotherUserIs404(t *testing.T) {
	e := setup(t, nil)
	e.play.sub = e.activePlay(42)
	e.mock.ExpectQuery(q("WHERE store_product_id_android = ?")).WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("FOR UPDATE")).WillReturnRows(subRows().AddRow(3, 77, 1, "google_play", "x", "active", true, fixedNow, fixedNow))
	e.mock.ExpectRollback()
	parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/verify",
		map[string]any{"store": "google_play", "productId": "plus_android", "purchaseToken": "tok-1"}, nil), 404)
	if len(e.events) != 0 {
		t.Fatal("no events on failure")
	}
}

func TestVerifyPlayStoreErrors(t *testing.T) {
	e := setup(t, nil)
	body := map[string]any{"store": "google_play", "productId": "plus_android", "purchaseToken": "tok-1"}
	e.play.err = errUpstream
	env := parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/verify", body, nil), 502)
	if env.Error.Code != "UPSTREAM_ERROR" {
		t.Fatal(env.Error.Code)
	}
	e.play.err = errNotFound
	parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/verify", body, nil), 404)
	e.play.err = nil
	e.play.sub = e.activePlay(42)
	e.play.sub.State = "SUBSCRIPTION_STATE_PENDING"
	env = parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/verify", body, nil), 409)
	if env.Error.Code != "INVALID_STATE" {
		t.Fatal(env.Error.Code)
	}
}

func TestVerifyStoreNotConfiguredIsInvalidState(t *testing.T) {
	d, _ := testutil.Deps(t, nil) // no google-play / app-store secret
	m := New(d)
	r, routes := testutil.UserEngine(testutil.VerifiedUser(42))
	m.Routes(routes)
	env := parse(t, testutil.Request(r, "POST", "/v1/premium/purchases/verify",
		map[string]any{"store": "google_play", "productId": "p", "purchaseToken": "t"}, nil), 409)
	if env.Error.Code != "INVALID_STATE" {
		t.Fatal(env.Error.Code)
	}
	parse(t, testutil.Request(r, "POST", "/v1/premium/purchases/verify",
		map[string]any{"store": "app_store", "productId": "p", "transactionId": "1000"}, nil), 409)
}

func TestVerifyPlayUpgradeCancelsLinkedPurchase(t *testing.T) {
	e := setup(t, nil)
	e.play.sub = e.activePlay(42)
	e.play.sub.LinkedPurchaseToken = "old-tok"
	e.play.sub.Acknowledged = true
	e.mock.ExpectQuery(q("WHERE store_product_id_android = ?")).WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("store_transaction_id = ? FOR UPDATE")).WillReturnRows(subRows())
	e.mock.ExpectExec(q("INSERT INTO subscriptions")).WillReturnResult(sqlmock.NewResult(8, 1))
	e.mock.ExpectQuery(q("purchase_token_hash = ? ORDER BY id DESC LIMIT 1 FOR UPDATE")).WithArgs(crypto.SHA256("old-tok")).
		WillReturnRows(subRows().AddRow(5, 42, 1, "google_play", "old", "active", true, fixedNow, fixedNow.Add(time.Hour)))
	e.mock.ExpectExec(q("UPDATE subscriptions SET status = ? WHERE id = ?")).WithArgs("cancelled", int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.expectRender(1)
	parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/verify",
		map[string]any{"store": "google_play", "productId": "plus_android", "purchaseToken": "new-tok"}, nil), 200)
	if got := e.eventTypes(); len(got) != 2 || got[0] != events.SubscriptionActivated || got[1] != events.SubscriptionExpired {
		t.Fatalf("events %v", got)
	}
	if e.play.acks != 1 {
		t.Fatal("ack expected")
	}
}

func (e *env) appleSub(userID int64, status int) AppleSubscription {
	return AppleSubscription{OriginalTransactionID: "1000", TransactionID: "1001", ProductID: "plus_ios",
		AppAccountToken: strings.ToUpper(storeAccountToken(e.d.Crypto, userID)), Status: status,
		StartedAt: fixedNow.Add(-time.Hour), ExpiresAt: fixedNow.Add(24 * time.Hour), AutoRenew: true}
}

func TestVerifyAppleHappyPathAndReverify(t *testing.T) {
	e := setup(t, nil)
	e.apple.subs = []AppleSubscription{{ProductID: "other_ios"}, e.appleSub(42, 1)}
	body := map[string]any{"store": "app_store", "productId": "plus_ios", "transactionId": "1001"}
	e.mock.ExpectQuery(q("WHERE store_product_id_ios = ?")).WithArgs("plus_ios").WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("FOR UPDATE")).WithArgs("app_store", "1000").WillReturnRows(subRows())
	e.mock.ExpectExec(q("INSERT INTO subscriptions")).
		WithArgs(int64(42), int64(1), "app_store", "1000", nil, "active", true, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(9, 1))
	e.mock.ExpectCommit()
	e.expectRender(1)
	parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/verify", body, nil), 200)

	// Same purchase again: the row is updated, no second activation event.
	e.mock.ExpectQuery(q("WHERE store_product_id_ios = ?")).WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("FOR UPDATE")).WillReturnRows(subRows().AddRow(9, 42, 1, "app_store", "1000", "active", true, fixedNow, fixedNow))
	e.mock.ExpectExec(q("UPDATE subscriptions SET plan_id = ?, status = ?, auto_renew = ?, expires_at = ? WHERE id = ?")).
		WithArgs(int64(1), "active", true, sqlmock.AnyArg(), int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.expectRender(1)
	parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/verify", body, nil), 200)
	if got := e.eventTypes(); len(got) != 1 {
		t.Fatalf("events %v", got)
	}
}

func TestVerifyAppleOtherAccountIs404(t *testing.T) {
	e := setup(t, nil)
	e.apple.subs = []AppleSubscription{e.appleSub(77, 1)}
	parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/purchases/verify",
		map[string]any{"store": "app_store", "productId": "plus_ios", "transactionId": "1001"}, nil), 404)
}

func TestAppleStatusMapping(t *testing.T) {
	e := setup(t, nil)
	s := e.m.svc
	grace := fixedNow.Add(48 * time.Hour)
	cases := []struct {
		a    AppleSubscription
		want string
	}{
		{AppleSubscription{Status: 1, ExpiresAt: fixedNow.Add(time.Hour)}, StatusActive},
		{AppleSubscription{Status: 1, ExpiresAt: fixedNow.Add(-time.Hour)}, StatusExpired},
		{AppleSubscription{Status: 4, ExpiresAt: fixedNow.Add(-time.Hour), GraceExpiresAt: &grace}, StatusGrace},
		{AppleSubscription{Status: 3, ExpiresAt: fixedNow.Add(time.Hour)}, StatusExpired},
		{AppleSubscription{Status: 1, Revoked: true, ExpiresAt: fixedNow.Add(time.Hour)}, StatusRefunded},
		{AppleSubscription{Status: 5}, StatusRefunded},
	}
	for i, c := range cases {
		if got := s.appleToState(c.a, 1).Status; got != c.want {
			t.Errorf("case %d: %s want %s", i, got, c.want)
		}
	}
	if playStatus("SUBSCRIPTION_STATE_CANCELED", fixedNow.Add(time.Hour), fixedNow) != StatusActive ||
		playStatus("SUBSCRIPTION_STATE_IN_GRACE_PERIOD", fixedNow.Add(time.Hour), fixedNow) != StatusGrace ||
		playStatus("SUBSCRIPTION_STATE_ON_HOLD", fixedNow.Add(time.Hour), fixedNow) != StatusExpired ||
		playStatus("SUBSCRIPTION_STATE_ACTIVE", fixedNow.Add(-time.Hour), fixedNow) != StatusExpired ||
		playStatus("SOMETHING_NEW", fixedNow.Add(time.Hour), fixedNow) != StatusExpired {
		t.Fatal("play status mapping")
	}
}

// ---- mock activate ----

func TestMockActivate(t *testing.T) {
	e := setup(t, map[string]any{"premium.enabled": true})
	e.mock.ExpectQuery(q("FROM premium_plans WHERE id = ?")).WithArgs(int64(1)).WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectExec(q("INSERT INTO subscriptions")).
		WithArgs(int64(42), int64(1), "manual", nil, nil, "active", false, fixedNow, fixedNow.AddDate(0, 1, 0)).
		WillReturnResult(sqlmock.NewResult(11, 1))
	e.expectRender(1)
	env := parse(t, testutil.Request(e.user(42), "POST", "/v1/premium/mock/activate", map[string]any{"planId": 1}, nil), 201)
	var sub Subscription
	_ = json.Unmarshal(env.Data, &sub)
	if sub.Store != "manual" || !sub.ExpiresAt.Equal(fixedNow.AddDate(0, 1, 0)) {
		t.Fatalf("sub %s", env.Data)
	}
	if got := e.eventTypes(); len(got) != 1 || got[0] != events.SubscriptionActivated {
		t.Fatalf("events %v", got)
	}
}

func TestMockActivateRules(t *testing.T) {
	e := setup(t, map[string]any{"premium.enabled": false})
	r := e.user(42)
	env := parse(t, testutil.Request(r, "POST", "/v1/premium/mock/activate", map[string]any{"planId": 1}, nil), 409)
	if env.Error.Code != "INVALID_STATE" {
		t.Fatal(env.Error.Code)
	}
	e.d.Settings.Set("premium.enabled", true)
	e.mock.ExpectQuery(q("FROM premium_plans WHERE id = ?")).WillReturnRows(addPlan(planRows(), 1, false))
	parse(t, testutil.Request(r, "POST", "/v1/premium/mock/activate", map[string]any{"planId": 1}, nil), 404)
	parse(t, testutil.Request(r, "POST", "/v1/premium/mock/activate", map[string]any{}, nil), 422)
	e.d.Cfg.Env = config.EnvProd
	parse(t, testutil.Request(r, "POST", "/v1/premium/mock/activate", map[string]any{"planId": 1}, nil), 404)
}

// ---- Google Play webhook ----

func pushBody(t *testing.T, messageID string, n any) map[string]any {
	b, _ := json.Marshal(n)
	return map[string]any{"message": map[string]any{"messageId": messageID, "data": base64.StdEncoding.EncodeToString(b)}, "subscription": "projects/x/subscriptions/y"}
}

func rtdnFor(typ int, token string) map[string]any {
	return map[string]any{"packageName": "app.athlix", "subscriptionNotification": map[string]any{"notificationType": typ, "purchaseToken": token}}
}

func TestPlayWebhookBadAuthIs401AndBodyNotRead(t *testing.T) {
	e := setup(t, nil)
	e.push.err = errBadSignature
	read := false
	err := e.m.svc.HandlePlayPush(context.Background(), "Bearer x", func() ([]byte, error) { read = true; return nil, nil })
	if he, ok := err.(interface{ Error() string }); !ok || !strings.Contains(he.Error(), "UNAUTHENTICATED") || read {
		t.Fatalf("err %v read %v", err, read)
	}
	env := parse(t, testutil.Request(e.user(1), "POST", "/v1/webhooks/google-play", pushBody(t, "m1", rtdnFor(2, "tok")), nil), 401)
	if env.Error.Code != "UNAUTHENTICATED" {
		t.Fatal(env.Error.Code)
	}
}

func TestPlayWebhookRenewalReactivates(t *testing.T) {
	e := setup(t, nil)
	e.play.sub = e.activePlay(42)
	h := crypto.SHA256("tok-1")
	e.mock.ExpectExec(q("INSERT INTO payment_webhook_events")).WithArgs("google_play", "m1", "SUBSCRIPTION_RENEWED").WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectQuery(q("purchase_token_hash = ?")).WithArgs(h).
		WillReturnRows(subRows().AddRow(4, 42, 1, "google_play", hexs(h), "expired", true, fixedNow.Add(-40*24*time.Hour), fixedNow.Add(-time.Hour)))
	e.mock.ExpectQuery(q("WHERE store_product_id_android = ?")).WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("store_transaction_id = ? FOR UPDATE")).WithArgs("google_play", hexs(h)).
		WillReturnRows(subRows().AddRow(4, 42, 1, "google_play", hexs(h), "expired", true, fixedNow.Add(-40*24*time.Hour), fixedNow.Add(-time.Hour)))
	e.mock.ExpectExec(q("UPDATE subscriptions SET plan_id = ?")).WithArgs(int64(1), "active", true, e.play.sub.ExpiresAt, int64(4)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.mock.ExpectExec(q("UPDATE payment_webhook_events SET status = ?, entity_id = ?, processed_at = ?")).
		WithArgs("processed", "4", fixedNow, "google_play", "m1").WillReturnResult(sqlmock.NewResult(0, 1))
	parse(t, testutil.Request(e.user(1), "POST", "/v1/webhooks/google-play", pushBody(t, "m1", rtdnFor(2, "tok-1")), map[string]string{"Authorization": "Bearer ok"}), 200)
	if got := e.eventTypes(); len(got) != 1 || got[0] != events.SubscriptionActivated {
		t.Fatalf("events %v", got)
	}
	if e.play.acks != 1 {
		t.Fatal("unacknowledged purchase must be acknowledged")
	}
}

func TestPlayWebhookDuplicateIsIdempotent(t *testing.T) {
	e := setup(t, nil)
	e.mock.ExpectExec(q("INSERT INTO payment_webhook_events")).WillReturnError(dupErr())
	e.mock.ExpectQuery(q("SELECT status FROM payment_webhook_events WHERE provider = ? AND provider_event_id = ?")).
		WithArgs("google_play", "m1").WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("processed"))
	parse(t, testutil.Request(e.user(1), "POST", "/v1/webhooks/google-play", pushBody(t, "m1", rtdnFor(2, "tok-1")), nil), 200)
	if e.play.calls != 0 || len(e.events) != 0 {
		t.Fatal("duplicate must not be reprocessed")
	}
}

func TestPlayWebhookFailedEventIsRetried(t *testing.T) {
	e := setup(t, nil)
	e.play.err = errUpstream
	e.mock.ExpectExec(q("INSERT INTO payment_webhook_events")).WillReturnError(dupErr())
	e.mock.ExpectQuery(q("SELECT status FROM payment_webhook_events")).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("failed"))
	e.mock.ExpectExec(q("UPDATE payment_webhook_events")).WithArgs("failed", nil, fixedNow, "google_play", "m1").WillReturnResult(sqlmock.NewResult(0, 1))
	parse(t, testutil.Request(e.user(1), "POST", "/v1/webhooks/google-play", pushBody(t, "m1", rtdnFor(2, "tok-1")), nil), 502)
	if e.play.calls != 1 {
		t.Fatal("failed event must be retried")
	}
}

func TestPlayWebhookIgnoredCases(t *testing.T) {
	e := setup(t, nil)
	r := e.user(1)
	// Other package.
	e.mock.ExpectExec(q("INSERT INTO payment_webhook_events")).WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectExec(q("UPDATE payment_webhook_events")).WithArgs("ignored", nil, fixedNow, "google_play", "m2").WillReturnResult(sqlmock.NewResult(0, 1))
	n := rtdnFor(2, "tok")
	n["packageName"] = "com.other"
	parse(t, testutil.Request(r, "POST", "/v1/webhooks/google-play", pushBody(t, "m2", n), nil), 200)
	// Test notification (no subscriptionNotification).
	e.mock.ExpectExec(q("INSERT INTO payment_webhook_events")).WithArgs("google_play", "m3", "other").WillReturnResult(sqlmock.NewResult(2, 1))
	e.mock.ExpectExec(q("UPDATE payment_webhook_events")).WithArgs("ignored", nil, fixedNow, "google_play", "m3").WillReturnResult(sqlmock.NewResult(0, 1))
	parse(t, testutil.Request(r, "POST", "/v1/webhooks/google-play", pushBody(t, "m3", map[string]any{"packageName": "app.athlix", "testNotification": map[string]any{}}), nil), 200)
	// Unknown token (no row) is ignored.
	e.play.sub = e.activePlay(42)
	e.mock.ExpectExec(q("INSERT INTO payment_webhook_events")).WillReturnResult(sqlmock.NewResult(3, 1))
	e.mock.ExpectQuery(q("purchase_token_hash = ?")).WillReturnRows(subRows())
	e.mock.ExpectExec(q("UPDATE payment_webhook_events")).WithArgs("ignored", nil, fixedNow, "google_play", "m4").WillReturnResult(sqlmock.NewResult(0, 1))
	parse(t, testutil.Request(r, "POST", "/v1/webhooks/google-play", pushBody(t, "m4", rtdnFor(4, "tok-x")), nil), 200)
	// Malformed body after auth.
	parse(t, testutil.Request(r, "POST", "/v1/webhooks/google-play", map[string]any{"message": map[string]any{"messageId": "m5", "data": "%%%"}}, nil), 400)
	if len(e.events) != 0 {
		t.Fatal("no events expected")
	}
}

func TestPlayWebhookRevokedMarksRefunded(t *testing.T) {
	e := setup(t, nil)
	e.play.sub = e.activePlay(42)
	e.play.sub.State, e.play.sub.Acknowledged = "SUBSCRIPTION_STATE_EXPIRED", true
	h := crypto.SHA256("tok-1")
	live := subRows().AddRow(4, 42, 1, "google_play", hexs(h), "active", true, fixedNow, fixedNow.Add(time.Hour))
	e.mock.ExpectExec(q("INSERT INTO payment_webhook_events")).WithArgs("google_play", "m9", "SUBSCRIPTION_REVOKED").WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectQuery(q("purchase_token_hash = ?")).WillReturnRows(live)
	e.mock.ExpectQuery(q("WHERE store_product_id_android = ?")).WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("store_transaction_id = ? FOR UPDATE")).
		WillReturnRows(subRows().AddRow(4, 42, 1, "google_play", hexs(h), "active", true, fixedNow, fixedNow.Add(time.Hour)))
	e.mock.ExpectExec(q("UPDATE subscriptions SET plan_id = ?")).WithArgs(int64(1), "refunded", false, sqlmock.AnyArg(), int64(4)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.mock.ExpectExec(q("UPDATE payment_webhook_events")).WithArgs("processed", "4", fixedNow, "google_play", "m9").WillReturnResult(sqlmock.NewResult(0, 1))
	parse(t, testutil.Request(e.user(1), "POST", "/v1/webhooks/google-play", pushBody(t, "m9", rtdnFor(12, "tok-1")), nil), 200)
	if got := e.eventTypes(); len(got) != 1 || got[0] != events.SubscriptionExpired {
		t.Fatalf("events %v", got)
	}
	if e.play.acks != 0 {
		t.Fatal("no ack for an already acknowledged purchase")
	}
}

// ---- App Store webhook ----

func TestAppleWebhookBadSignatureIs401(t *testing.T) {
	e := setup(t, nil)
	e.apple.noteErr = errBadSignature
	r := e.user(1)
	parse(t, testutil.Request(r, "POST", "/v1/webhooks/app-store", map[string]any{"signedPayload": "a.b.c"}, nil), 401)
	parse(t, testutil.Request(r, "POST", "/v1/webhooks/app-store", map[string]any{"nope": 1}, nil), 401)
}

func TestAppleWebhookExpiredAndDuplicate(t *testing.T) {
	e := setup(t, nil)
	e.apple.note = AppleNotification{UUID: "n-1", Type: "EXPIRED.VOLUNTARY", TransactionID: "1001", OriginalTransactionID: "1000", Ours: true}
	a := e.appleSub(42, 2)
	a.ExpiresAt, a.AutoRenew = fixedNow.Add(-time.Hour), false
	e.apple.subs = []AppleSubscription{a}
	live := func() *sqlmock.Rows {
		return subRows().AddRow(9, 42, 1, "app_store", "1000", "active", true, fixedNow.Add(-24*time.Hour), fixedNow.Add(-time.Hour))
	}
	e.mock.ExpectExec(q("INSERT INTO payment_webhook_events")).WithArgs("app_store", "n-1", "EXPIRED.VOLUNTARY").WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectQuery(q("store_transaction_id = ? FOR UPDATE")).WithArgs("app_store", "1000").WillReturnRows(live())
	e.mock.ExpectQuery(q("WHERE store_product_id_ios = ?")).WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("store_transaction_id = ? FOR UPDATE")).WillReturnRows(live())
	e.mock.ExpectExec(q("UPDATE subscriptions SET plan_id = ?")).WithArgs(int64(1), "expired", false, sqlmock.AnyArg(), int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.mock.ExpectExec(q("UPDATE payment_webhook_events")).WithArgs("processed", "9", fixedNow, "app_store", "n-1").WillReturnResult(sqlmock.NewResult(0, 1))
	r := e.user(1)
	parse(t, testutil.Request(r, "POST", "/v1/webhooks/app-store", map[string]any{"signedPayload": "a.b.c"}, nil), 200)
	if got := e.eventTypes(); len(got) != 1 || got[0] != events.SubscriptionExpired {
		t.Fatalf("events %v", got)
	}
	// Redelivery of the same notification: 200, nothing re-fetched.
	e.mock.ExpectExec(q("INSERT INTO payment_webhook_events")).WillReturnError(dupErr())
	e.mock.ExpectQuery(q("SELECT status FROM payment_webhook_events")).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("processed"))
	parse(t, testutil.Request(r, "POST", "/v1/webhooks/app-store", map[string]any{"signedPayload": "a.b.c"}, nil), 200)
	if e.apple.calls != 1 || len(e.events) != 1 {
		t.Fatal("duplicate must not be reprocessed")
	}
}

func TestAppleWebhookNotOursIgnored(t *testing.T) {
	e := setup(t, nil)
	e.apple.note = AppleNotification{UUID: "n-2", Type: "TEST", Ours: false}
	e.mock.ExpectExec(q("INSERT INTO payment_webhook_events")).WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectExec(q("UPDATE payment_webhook_events")).WithArgs("ignored", nil, fixedNow, "app_store", "n-2").WillReturnResult(sqlmock.NewResult(0, 1))
	parse(t, testutil.Request(e.user(1), "POST", "/v1/webhooks/app-store", map[string]any{"signedPayload": "a.b.c"}, nil), 200)
	if e.apple.calls != 0 {
		t.Fatal("must not call the store")
	}
}

// ---- admin ----

func TestAdminPermissions(t *testing.T) {
	e := setup(t, nil)
	r := e.admin("users.read")
	for _, c := range []struct{ method, path string }{
		{"GET", "/v1/admin/plans"}, {"POST", "/v1/admin/plans"}, {"PUT", "/v1/admin/plans/1"},
		{"PUT", "/v1/admin/plans/1/benefits"}, {"GET", "/v1/admin/subscriptions"}, {"POST", "/v1/admin/subscriptions/grant"},
	} {
		parse(t, testutil.Request(r, c.method, c.path, map[string]any{}, nil), 403)
	}
	// payments.read may list subscriptions but not grant.
	r = e.admin("payments.read")
	parse(t, testutil.Request(r, "POST", "/v1/admin/subscriptions/grant", map[string]any{}, nil), 403)
}

func TestAdminListPlansIncludesInactive(t *testing.T) {
	e := setup(t, nil)
	e.mock.ExpectQuery(q("FROM premium_plans WHERE 1=1 ORDER BY sort_order, id")).
		WillReturnRows(addPlan(addPlan(planRows(), 1, true), 2, false))
	e.mock.ExpectQuery(q("FROM premium_plan_benefits WHERE plan_id IN (?,?)")).WithArgs(int64(1), int64(2)).WillReturnRows(benefitRows())
	env := parse(t, testutil.Request(e.admin("config.read"), "GET", "/v1/admin/plans", nil, nil), 200)
	var plans []AdminPlan
	_ = json.Unmarshal(env.Data, &plans)
	if len(plans) != 2 || plans[1].Active || plans[0].SortOrder != 1 || plans[0].Benefits == nil {
		t.Fatalf("plans %s", env.Data)
	}
}

func TestAdminCreatePlan(t *testing.T) {
	e := setup(t, nil)
	r := e.admin("plans.write")
	env := parse(t, testutil.Request(r, "POST", "/v1/admin/plans", map[string]any{"code": "Bad Code", "audience": "fan", "billingPeriod": "weekly", "pricePaise": -1}, nil), 422)
	for _, k := range []string{"code", "audience", "name", "billingPeriod", "pricePaise"} {
		if env.Error.Fields[k] == "" {
			t.Errorf("missing field error %s: %v", k, env.Error.Fields)
		}
	}
	body := map[string]any{"code": "coach_pro_yearly", "audience": "coach", "name": "Coach Pro", "pricePaise": 299000,
		"billingPeriod": "yearly", "storeProductIdAndroid": "cp_year"}
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("SELECT COUNT(*) FROM premium_plans WHERE store_product_id_android = ? AND id <> ?")).WithArgs("cp_year", int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	e.mock.ExpectExec(q("INSERT INTO premium_plans")).
		WithArgs("coach_pro_yearly", "coach", "Coach Pro", nil, int64(299000), "yearly", "cp_year", nil, true, 0).
		WillReturnResult(sqlmock.NewResult(5, 1))
	e.mock.ExpectExec(q("INSERT INTO config_audit")).WithArgs("premium_plans", "5", nil, sqlmock.AnyArg(), int64(9)).WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectExec(q("INSERT INTO admin_audit_log")).WithArgs(int64(9), "plan.create", "premium_plan", "5", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectCommit()
	env = parse(t, testutil.Request(r, "POST", "/v1/admin/plans", body, nil), 201)
	var p AdminPlan
	_ = json.Unmarshal(env.Data, &p)
	if p.ID != 5 || !p.Active || p.Code != "coach_pro_yearly" || string(mustJSON(p.Benefits)) != "[]" {
		t.Fatalf("plan %s", env.Data)
	}
	// Duplicate code.
	body["storeProductIdAndroid"] = nil
	e.mock.ExpectBegin()
	e.mock.ExpectExec(q("INSERT INTO premium_plans")).WillReturnError(dupErr())
	e.mock.ExpectRollback()
	env = parse(t, testutil.Request(r, "POST", "/v1/admin/plans", body, nil), 409)
	if env.Error.Code != "CONFLICT" {
		t.Fatal(env.Error.Code)
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestAdminUpdatePlan(t *testing.T) {
	e := setup(t, nil)
	r := e.admin("plans.write")
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("FROM premium_plans WHERE id = ?")).WithArgs(int64(404)).WillReturnRows(planRows())
	e.mock.ExpectRollback()
	parse(t, testutil.Request(r, "PUT", "/v1/admin/plans/404", map[string]any{"name": "X"}, nil), 404)

	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("FROM premium_plans WHERE id = ?")).WithArgs(int64(1)).WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectQuery(q("WHERE store_product_id_android = ? AND id <> ?")).WithArgs("plus_android", int64(1)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	e.mock.ExpectQuery(q("WHERE store_product_id_ios = ? AND id <> ?")).WithArgs("plus_ios", int64(1)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	e.mock.ExpectRollback()
	parse(t, testutil.Request(r, "PUT", "/v1/admin/plans/1", map[string]any{"name": "X"}, nil), 409)

	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("FROM premium_plans WHERE id = ?")).WithArgs(int64(1)).WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectExec(q("UPDATE premium_plans SET")).
		WithArgs("player_plus_monthly", "player", "Player Plus", nil, int64(12900), "monthly", nil, nil, false, 1, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q("INSERT INTO config_audit")).WithArgs("premium_plans", "1", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(9)).WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectExec(q("INSERT INTO admin_audit_log")).WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectQuery(q("FROM premium_plan_benefits")).WillReturnRows(benefitRows())
	e.mock.ExpectCommit()
	env := parse(t, testutil.Request(r, "PUT", "/v1/admin/plans/1", map[string]any{"pricePaise": 12900, "active": false,
		"description": "", "storeProductIdAndroid": "", "storeProductIdIos": ""}, nil), 200)
	var p AdminPlan
	_ = json.Unmarshal(env.Data, &p)
	if p.PricePaise != 12900 || p.Active || p.Description != nil || p.Name != "Player Plus" {
		t.Fatalf("plan %s", env.Data)
	}
}

func TestAdminReplaceBenefits(t *testing.T) {
	e := setup(t, nil)
	r := e.admin("plans.write")
	env := parse(t, testutil.Request(r, "PUT", "/v1/admin/plans/1/benefits", []map[string]any{
		{"key": "free_money", "value": true, "label": "x"},
		{"key": "featured_events", "value": 0, "label": "x"},
		{"key": "profile_boost", "value": 3, "label": ""},
	}, nil), 422)
	for _, k := range []string{"benefits[0].key", "benefits[1].value", "benefits[2].value", "benefits[2].label"} {
		if env.Error.Fields[k] == "" {
			t.Errorf("missing %s: %v", k, env.Error.Fields)
		}
	}
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("FROM premium_plans WHERE id = ?")).WithArgs(int64(3)).WillReturnRows(addPlan(planRows(), 3, true))
	e.mock.ExpectQuery(q("FROM premium_plan_benefits")).WithArgs(int64(3)).WillReturnRows(benefitRows().AddRow(3, "featured_events", []byte("2"), "2 per month", 1))
	e.mock.ExpectExec(q("DELETE FROM premium_plan_benefits WHERE plan_id = ?")).WithArgs(int64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q("INSERT INTO premium_plan_benefits")).WithArgs(int64(3), "featured_events", "5", "5 per month", 1).WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectExec(q("INSERT INTO premium_plan_benefits")).WithArgs(int64(3), "verified_coach_badge", "true", "Badge", 2).WillReturnResult(sqlmock.NewResult(2, 1))
	e.mock.ExpectExec(q("INSERT INTO config_audit")).WithArgs("premium_plan_benefits", "3", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(9)).WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectExec(q("INSERT INTO admin_audit_log")).WithArgs(int64(9), "plan.benefits_replace", "premium_plan", "3", nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectCommit()
	env = parse(t, testutil.Request(r, "PUT", "/v1/admin/plans/3/benefits", []map[string]any{
		{"key": "featured_events", "value": 5, "label": "5 per month", "sortOrder": 1},
		{"key": "verified_coach_badge", "value": true, "label": "Badge", "sortOrder": 2},
	}, nil), 200)
	var p AdminPlan
	_ = json.Unmarshal(env.Data, &p)
	if len(p.Benefits) != 2 || string(p.Benefits[0].Value) != "5" {
		t.Fatalf("plan %s", env.Data)
	}
}

func TestAdminSubscriptionsPage(t *testing.T) {
	e := setup(t, nil)
	r := e.admin("payments.read")
	parse(t, testutil.Request(r, "GET", "/v1/admin/subscriptions?status=weird", nil, nil), 422)
	e.mock.ExpectQuery(q("FROM subscriptions WHERE user_id = ? AND status = ? ORDER BY id DESC LIMIT ?")).WithArgs(int64(42), "active", 2).
		WillReturnRows(subRows().
			AddRow(20, 42, 1, "manual", nil, "active", false, fixedNow, fixedNow.Add(time.Hour)).
			AddRow(19, 42, 1, "manual", nil, "active", false, fixedNow, fixedNow.Add(time.Hour)))
	e.expectRender(1)
	e.mock.ExpectQuery(q("FROM users u")).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "dn", "pr", "vs", "photo", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"}).
		AddRow(42, "Asha", "player", "verified", nil, "Pune", "MH", nil, nil, nil, nil, nil, nil))
	env := parse(t, testutil.Request(r, "GET", "/v1/admin/subscriptions?userId=42&status=active&limit=1", nil, nil), 200)
	var items []AdminSubscription
	_ = json.Unmarshal(env.Data, &items)
	if len(items) != 1 || items[0].ID != 20 || items[0].User == nil || items[0].User.DisplayName != "Asha" {
		t.Fatalf("items %s", env.Data)
	}
	if env.Meta["nextCursor"] == nil {
		t.Fatal("expected next cursor")
	}
}

func TestAdminGrant(t *testing.T) {
	e := setup(t, nil)
	r := e.admin("plans.write")
	parse(t, testutil.Request(r, "POST", "/v1/admin/subscriptions/grant", map[string]any{"userId": 42, "planId": 1, "days": 0}, nil), 422)
	userRow := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "dn", "pr", "vs", "photo", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"}).
			AddRow(42, "Asha", "player", "verified", nil, nil, nil, nil, nil, nil, nil, nil, nil)
	}
	body := map[string]any{"userId": 42, "planId": 1, "days": 30, "reason": "Tournament prize"}
	e.mock.ExpectQuery(q("FROM users u")).WillReturnRows(sqlmock.NewRows([]string{"id", "dn", "pr", "vs", "photo", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"}))
	env := parse(t, testutil.Request(r, "POST", "/v1/admin/subscriptions/grant", body, nil), 404)
	if env.Error.Code != "NOT_FOUND" {
		t.Fatal(env.Error.Code)
	}
	e.mock.ExpectQuery(q("FROM users u")).WillReturnRows(userRow())
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("FROM premium_plans WHERE id = ?")).WithArgs(int64(1)).WillReturnRows(addPlan(planRows(), 1, true))
	e.mock.ExpectExec(q("INSERT INTO subscriptions")).
		WithArgs(int64(42), int64(1), "manual", nil, nil, "active", false, fixedNow, fixedNow.AddDate(0, 0, 30)).WillReturnResult(sqlmock.NewResult(12, 1))
	e.mock.ExpectExec(q("INSERT INTO admin_audit_log")).WithArgs(int64(9), "subscription.grant", "subscription", "12", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectCommit()
	e.expectRender(1)
	env = parse(t, testutil.Request(r, "POST", "/v1/admin/subscriptions/grant", body, nil), 201)
	var sub Subscription
	_ = json.Unmarshal(env.Data, &sub)
	if sub.ID != 12 || sub.Store != "manual" || sub.AutoRenew {
		t.Fatalf("sub %s", env.Data)
	}
	if got := e.eventTypes(); len(got) != 1 || got[0] != events.SubscriptionActivated {
		t.Fatalf("events %v", got)
	}
}

// ---- job ----

func TestExpireJob(t *testing.T) {
	e := setup(t, nil)
	job := e.m.Jobs()["subscription.expire"]
	if job == nil {
		t.Fatal("job not registered")
	}
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("WHERE status IN ('active','grace') AND expires_at <= ? ORDER BY id LIMIT ? FOR UPDATE")).WithArgs(fixedNow, jobBatch).
		WillReturnRows(subRows().
			AddRow(1, 42, 1, "manual", nil, "active", false, fixedNow, fixedNow.Add(-time.Hour)).
			AddRow(2, 43, 2, "google_play", "t", "grace", true, fixedNow, fixedNow.Add(-time.Minute)))
	e.mock.ExpectExec(q("UPDATE subscriptions SET status = 'expired' WHERE id IN (?,?) AND status IN ('active','grace') AND expires_at <= ?")).
		WithArgs(int64(1), int64(2), fixedNow).WillReturnResult(sqlmock.NewResult(0, 2))
	e.mock.ExpectCommit()
	if err := job(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := e.eventTypes(); len(got) != 2 || got[0] != events.SubscriptionExpired {
		t.Fatalf("events %v", got)
	}
	// Re-run with nothing due: no update, no events.
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("expires_at <= ?")).WillReturnRows(subRows())
	e.mock.ExpectCommit()
	if err := job(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.events) != 2 {
		t.Fatal("re-run must not emit")
	}
}

func TestJobFailureRollsBack(t *testing.T) {
	e := setup(t, nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q("expires_at <= ?")).WillReturnError(errors.New("db down"))
	e.mock.ExpectRollback()
	if err := e.m.svc.ExpireDue(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if len(e.events) != 0 {
		t.Fatal("no events on failure")
	}
}

var _ http.Handler = (*gin.Engine)(nil)

package listingpayment

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/config"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/razorpay"
	"athlix/backend/internal/testutil"
)

var fixedNow = time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)

const (
	uid     = int64(7)
	eventID = int64(10)
	orderID = int64(5)
)

// ---------- harness ----------

type env struct {
	t    *testing.T
	d    *app.Deps
	mock sqlmock.Sqlmock
	m    *Module
	rz   *razorpay.Fake
	got  map[string][]events.Event
}

func newEnv(t *testing.T, cfg map[string]any) *env {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return fixedNow }
	e := &env{t: t, d: d, mock: mock, m: New(d), got: map[string][]events.Event{},
		rz: &razorpay.Fake{Key: "rzp_test_k", KeySecret: "ks", WebhookSecret: "ws"}}
	e.m.svc.rz = func(context.Context) (razorpay.Client, error) { return e.rz, nil }
	for _, typ := range []string{events.ListingPaid, events.ListingRefunded, events.EventSubmitted, events.EventPublished} {
		typ := typ
		d.Dispatcher.On(typ, func(_ context.Context, ev events.Event) error {
			e.got[typ] = append(e.got[typ], ev)
			return nil
		})
	}
	return e
}

func (e *env) user(id int64) http.Handler {
	r, routes := testutil.UserEngine(testutil.VerifiedUser(id))
	e.m.Routes(routes)
	return r
}

func (e *env) admin(perms ...string) http.Handler {
	r, routes := testutil.AdminEngine(9, perms)
	e.m.Routes(routes)
	return r
}

func (e *env) published(typ string) []events.ListingPayload {
	var out []events.ListingPayload
	for _, ev := range e.got[typ] {
		var p events.ListingPayload
		if err := ev.Decode(&p); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, p)
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

func decode(t *testing.T, w *httptest.ResponseRecorder, code int, out any) envelope {
	t.Helper()
	if w.Code != code {
		t.Fatalf("status %d, want %d: %s", w.Code, code, w.Body)
	}
	var en envelope
	if err := json.Unmarshal(w.Body.Bytes(), &en); err != nil {
		t.Fatalf("bad json %s: %v", w.Body, err)
	}
	if out != nil {
		if err := json.Unmarshal(en.Data, out); err != nil {
			t.Fatalf("bad data %s: %v", en.Data, err)
		}
	}
	return en
}

func i64(v int64) *int64   { return &v }
func str(v string) *string { return &v }

// ---------- row builders ----------

var evColNames = []string{"id", "organiser_id", "sport_id", "event_type", "entry_fee_paise", "max_slots", "status",
	"starts_at", "ends_at", "registration_deadline", "participant_type", "team_size_min", "team_size_max", "registrations_count"}

type evOpt struct {
	status string
	fee    int64
	regs   int
	starts time.Time
}

func evRows(o evOpt) *sqlmock.Rows {
	if o.starts.IsZero() {
		o.starts = fixedNow.Add(72 * time.Hour)
	}
	return sqlmock.NewRows(evColNames).AddRow(eventID, uid, int64(4), "tournament", o.fee, int64(16), o.status,
		o.starts, o.starts.Add(4*time.Hour), o.starts.Add(-24*time.Hour), "individual", nil, nil, o.regs)
}

var ruleColNames = []string{"id", "name", "sport_id", "event_type", "percent_bp", "min_fee_paise", "max_fee_paise",
	"free_event_fee_paise", "gst_bp", "priority", "active", "effective_from", "effective_to"}

func defaultRule(rows *sqlmock.Rows) *sqlmock.Rows {
	return rows.AddRow(int64(1), "Default", nil, nil, int64(500), int64(9900), int64(499900), int64(0), int64(1800),
		int64(0), true, fixedNow.Add(-24*time.Hour), nil)
}

func sportRule(rows *sqlmock.Rows) *sqlmock.Rows {
	return rows.AddRow(int64(2), "Cricket", int64(4), nil, int64(400), int64(9900), nil, int64(0), int64(1800),
		int64(0), true, fixedNow.Add(-24*time.Hour), nil)
}

var orderColNames = []string{"id", "event_id", "organiser_id", "fee_rule_id", "entry_fee_paise", "max_slots", "percent_bp",
	"gst_bp", "base_fee_paise", "gst_paise", "total_paise", "provider", "provider_order_id", "provider_payment_id",
	"status", "refund_paise", "provider_refund_id", "paid_at", "expires_at", "created_at", "rule_name", "min_fee", "max_fee",
	"event_status"}

// stdOrder is 500bp of 50000 × 16 = 40000 + 18% GST 7200 = 47200.
func stdOrder(provider, status string) orderRec {
	o := orderRec{ID: orderID, EventID: eventID, OrganiserID: uid, FeeRuleID: 1, EntryFeePaise: 50000, MaxSlots: 16,
		PercentBp: 500, GstBp: 1800, BaseFeePaise: 40000, GstPaise: 7200, TotalPaise: 47200, Provider: provider,
		Status: status, ExpiresAt: fixedNow.Add(20 * time.Minute), CreatedAt: fixedNow.Add(-10 * time.Minute),
		RuleName: "Default", MinFeePaise: 9900, MaxFeePaise: i64(499900), EventStatus: "pending_payment"}
	if provider == providerRazorpay {
		o.ProviderOrderID = str("order_x")
	}
	if status == stPaid {
		t := fixedNow.Add(-5 * time.Minute)
		o.PaidAt = &t
		if provider == providerRazorpay {
			o.ProviderPaymentID = str("pay_abc")
		}
	}
	return o
}

func orderRows(os ...orderRec) *sqlmock.Rows {
	rows := sqlmock.NewRows(orderColNames)
	for _, o := range os {
		rows.AddRow(o.ID, o.EventID, o.OrganiserID, o.FeeRuleID, o.EntryFeePaise, o.MaxSlots, o.PercentBp, o.GstBp,
			o.BaseFeePaise, o.GstPaise, o.TotalPaise, o.Provider, ptrVal(o.ProviderOrderID), ptrVal(o.ProviderPaymentID),
			o.Status, o.RefundPaise, ptrVal(o.ProviderRefundID), timeVal(o.PaidAt), o.ExpiresAt, o.CreatedAt, o.RuleName,
			o.MinFeePaise, i64Val(o.MaxFeePaise), o.EventStatus)
	}
	return rows
}

func ptrVal(p *string) driver.Value {
	if p == nil {
		return nil
	}
	return *p
}

func timeVal(p *time.Time) driver.Value {
	if p == nil {
		return nil
	}
	return *p
}

func i64Val(p *int64) driver.Value {
	if p == nil {
		return nil
	}
	return *p
}

// ---------- SQL expectations ----------

func (e *env) expEvent(owner int64, lock bool, rows *sqlmock.Rows) {
	re := `FROM events e WHERE e.id = \?`
	args := []driver.Value{eventID}
	if owner > 0 {
		re += ` AND e.organiser_id = \?`
		args = append(args, owner)
	}
	if lock {
		re += ` FOR UPDATE`
	}
	e.mock.ExpectQuery(re + `$`).WithArgs(args...).WillReturnRows(rows)
}

func (e *env) expRules(rows *sqlmock.Rows) {
	e.mock.ExpectQuery(`FROM listing_fee_rules WHERE active = 1 AND effective_from <= \?`).
		WithArgs(fixedNow, fixedNow, int64(4), "tournament").WillReturnRows(rows)
}

func (e *env) expPolicyDoc(id any) {
	rows := sqlmock.NewRows([]string{"id"})
	if id != nil {
		rows.AddRow(id)
	}
	e.mock.ExpectQuery(`FROM policy_documents WHERE doc_type = 'listing_refund' AND active = 1`).WillReturnRows(rows)
}

func (e *env) expEventOrders(rows *sqlmock.Rows, statuses ...string) {
	re := `WHERE o.event_id = \? AND o.status IN \(` + strings.TrimSuffix(strings.Repeat(`\?,`, len(statuses)), ",") +
		`\) ORDER BY o.id ASC FOR UPDATE OF o`
	args := []driver.Value{eventID}
	for _, s := range statuses {
		args = append(args, s)
	}
	e.mock.ExpectQuery(re).WithArgs(args...).WillReturnRows(rows)
}

func (e *env) expSetStatus(to string, from ...string) {
	re := `UPDATE events SET status = \?`
	args := []driver.Value{to}
	if to == "published" {
		re += `, published_at = COALESCE\(published_at, \?\)`
		args = append(args, fixedNow)
	}
	re += ` WHERE id = \? AND status IN \(` + strings.TrimSuffix(strings.Repeat(`\?,`, len(from)), ",") + `\)`
	args = append(args, eventID)
	for _, f := range from {
		args = append(args, f)
	}
	e.mock.ExpectExec(re).WithArgs(args...).WillReturnResult(sqlmock.NewResult(0, 1))
}

func (e *env) expOrderByID(lock bool, rows *sqlmock.Rows) {
	re := `JOIN events e ON e.id = o.event_id WHERE o.id = \?`
	if lock {
		re += ` FOR UPDATE OF o`
	}
	e.mock.ExpectQuery(re + `$`).WithArgs(orderID).WillReturnRows(rows)
}

func (e *env) expMarkPaid(paymentID string) {
	e.mock.ExpectExec(`UPDATE listing_orders SET status = 'paid', provider_payment_id = \?, paid_at = \? WHERE id = \? AND status IN \('created','failed','expired'\)`).
		WithArgs(paymentID, fixedNow, orderID).WillReturnResult(sqlmock.NewResult(0, 1))
}

func (e *env) expAddRefund(amount int64, status string, refundID any, reason string, refundedAt any, prev int64) {
	e.mock.ExpectExec(`UPDATE listing_orders SET refund_paise = refund_paise \+ \?, status = \?`).
		WithArgs(amount, status, refundID, reason, refundedAt, orderID, prev).WillReturnResult(sqlmock.NewResult(0, 1))
}

var orderable3 = []string{"draft", "rejected", "pending_payment"}

// ---------- pure functions ----------

func TestCompute(t *testing.T) {
	r := FeeRule{ID: 1, Name: "Default", PercentBp: 500, MinFeePaise: 9900, MaxFeePaise: i64(499900), FreeEventFeePaise: 4900, GstBp: 1800}
	cases := []struct {
		name             string
		rule             FeeRule
		fee, slots       int64
		base, gst, total int64
		free             bool
	}{
		{"percent", r, 50000, 16, 40000, 7200, 47200, false},
		{"min clamp", r, 1000, 2, 9900, 1782, 11682, false},
		{"max clamp", r, 1000000, 1024, 499900, 89982, 589882, false},
		{"free event", r, 0, 64, 4900, 882, 5782, true},
		{"nil max is no cap", FeeRule{PercentBp: 500, GstBp: 1800}, 1000000, 1024, 51200000, 9216000, 60416000, false},
		{"half-up rounding", FeeRule{PercentBp: 1, GstBp: 1800}, 5000, 1, 1, 0, 1, false}, // 0.5 → 1, 0.18 → 0
		{"gst half-up", FeeRule{PercentBp: 10000, GstBp: 5000}, 1, 1, 1, 1, 2, false},     // 0.5 → 1
	}
	for _, c := range cases {
		q := compute(c.rule, c.fee, c.slots)
		if q.BaseFeePaise != c.base || q.GstPaise != c.gst || q.TotalPaise != c.total || q.IsFreeEvent != c.free {
			t.Errorf("%s: got base %d gst %d total %d free %v", c.name, q.BaseFeePaise, q.GstPaise, q.TotalPaise, q.IsFreeEvent)
		}
		if q.GrossEntryPaise != c.fee*c.slots {
			t.Errorf("%s: gross %d", c.name, q.GrossEntryPaise)
		}
	}
}

func TestPick(t *testing.T) {
	tp := "tournament"
	def := FeeRule{ID: 1}
	defHigh := FeeRule{ID: 2, Priority: 5}
	typ := FeeRule{ID: 3, EventType: &tp}
	sport := FeeRule{ID: 4, SportID: i64(4)}
	both := FeeRule{ID: 5, SportID: i64(4), EventType: &tp}
	if _, ok := pick(nil); ok {
		t.Fatal("empty list picked a rule")
	}
	for _, c := range []struct {
		rules []FeeRule
		want  int64
	}{
		{[]FeeRule{def, typ, sport, both}, 5},
		{[]FeeRule{def, typ, sport}, 4},
		{[]FeeRule{def, typ}, 3},
		{[]FeeRule{def, defHigh}, 2},
		{[]FeeRule{{ID: 7}, {ID: 9}, {ID: 8}}, 9},
	} {
		if got, _ := pick(c.rules); got.ID != c.want {
			t.Errorf("pick %v = %d, want %d", c.rules, got.ID, c.want)
		}
	}
}

func TestPercentOf(t *testing.T) {
	for _, c := range []struct {
		amount int64
		pct    int
		want   int64
	}{{47200, 100, 47200}, {47200, 50, 23600}, {47200, 0, 0}, {47200, -5, 0}, {47200, 150, 47200}, {1, 50, 1}, {3, 33, 1}} {
		if got := percentOf(c.amount, c.pct); got != c.want {
			t.Errorf("percentOf(%d, %d) = %d, want %d", c.amount, c.pct, got, c.want)
		}
	}
}

func TestParseTime(t *testing.T) {
	if v, ok := parseTime("2026-11-01", false); !ok || !v.Equal(time.Date(2026, 10, 31, 18, 30, 0, 0, time.UTC)) {
		t.Fatalf("date: %v %v", v, ok)
	}
	if v, ok := parseTime("2026-11-01", true); !ok || !v.Equal(time.Date(2026, 11, 1, 18, 30, 0, 0, time.UTC)) {
		t.Fatalf("end of day: %v %v", v, ok)
	}
	if v, ok := parseTime("2026-11-01T10:00:00Z", true); !ok || !v.Equal(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("rfc3339: %v %v", v, ok)
	}
	if v, ok := parseTime("", false); !ok || v != nil {
		t.Fatal("empty should be nil, ok")
	}
	if _, ok := parseTime("yesterday", false); ok {
		t.Fatal("bad value accepted")
	}
}

// ---------- quote ----------

func TestQuoteHappyPath(t *testing.T) {
	e := newEnv(t, nil)
	e.expRules(defaultRule(sqlmock.NewRows(ruleColNames)))
	e.expPolicyDoc(int64(3))
	var q Quote
	decode(t, testutil.Request(e.user(uid), "GET", "/v1/listing/quote?sportId=4&eventType=tournament&entryFeePaise=50000&maxSlots=16", nil, nil), 200, &q)
	if q.TotalPaise != 47200 || q.BaseFeePaise != 40000 || q.GstPaise != 7200 || q.GrossEntryPaise != 800000 || q.FeeRuleID != 1 {
		t.Fatalf("quote %+v", q)
	}
	if q.RefundPolicy != (RefundPolicy{100, 0, 100}) || q.RefundPolicyDocumentID == nil || *q.RefundPolicyDocumentID != 3 {
		t.Fatalf("policy %+v %v", q.RefundPolicy, q.RefundPolicyDocumentID)
	}
}

func TestQuoteUsesRefundSettingsAndMostSpecificRule(t *testing.T) {
	e := newEnv(t, map[string]any{"listing.refund.before_first_registration_percent": 90,
		"listing.refund.after_registrations_percent": 25, "listing.refund.on_admin_rejection_percent": 80})
	e.expRules(sportRule(defaultRule(sqlmock.NewRows(ruleColNames))))
	e.expPolicyDoc(nil)
	var q Quote
	decode(t, testutil.Request(e.user(uid), "GET", "/v1/listing/quote?sportId=4&eventType=tournament&entryFeePaise=50000&maxSlots=16", nil, nil), 200, &q)
	if q.FeeRuleID != 2 || q.RuleName != "Cricket" || q.BaseFeePaise != 32000 || q.MaxFeePaise != nil {
		t.Fatalf("quote %+v", q)
	}
	if q.RefundPolicy != (RefundPolicy{90, 25, 80}) || q.RefundPolicyDocumentID != nil {
		t.Fatalf("policy %+v", q.RefundPolicy)
	}
}

func TestQuoteNoRuleIs409(t *testing.T) {
	e := newEnv(t, nil)
	e.expRules(sqlmock.NewRows(ruleColNames))
	en := decode(t, testutil.Request(e.user(uid), "GET", "/v1/listing/quote?sportId=4&eventType=tournament&entryFeePaise=50000&maxSlots=16", nil, nil), 409, nil)
	if en.Error.Code != "INVALID_STATE" {
		t.Fatalf("code %s", en.Error.Code)
	}
}

func TestQuoteValidation(t *testing.T) {
	e := newEnv(t, nil)
	en := decode(t, testutil.Request(e.user(uid), "GET", "/v1/listing/quote?eventType=party&entryFeePaise=100&maxSlots=2000", nil, nil), 422, nil)
	for _, f := range []string{"sportId", "eventType", "maxSlots"} {
		if en.Error.Fields[f] == "" {
			t.Errorf("missing field error %s: %v", f, en.Error.Fields)
		}
	}
	decode(t, testutil.Request(e.user(uid), "GET", "/v1/listing/quote?sportId=-1", nil, nil), 422, nil)
}

func TestQuoteConfigRules(t *testing.T) {
	e := newEnv(t, map[string]any{"event.allow_free_events": false, "event.max_slots_limit": 8})
	en := decode(t, testutil.Request(e.user(uid), "GET", "/v1/listing/quote?sportId=4&eventType=tournament&entryFeePaise=0&maxSlots=16", nil, nil), 422, nil)
	if en.Error.Fields["entryFeePaise"] == "" || en.Error.Fields["maxSlots"] != "must be between 1 and 8" {
		t.Fatalf("fields %v", en.Error.Fields)
	}
}

func TestEventQuote(t *testing.T) {
	e := newEnv(t, nil)
	e.expEvent(uid, false, evRows(evOpt{status: "draft", fee: 50000}))
	e.expRules(defaultRule(sqlmock.NewRows(ruleColNames)))
	e.expPolicyDoc(nil)
	var q Quote
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/quote", nil, nil), 200, &q)
	if q.TotalPaise != 47200 {
		t.Fatalf("quote %+v", q)
	}
}

func TestEventQuoteNotOwnerIs404(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectQuery(`FROM events e WHERE e.id = \? AND e.organiser_id = \?$`).WithArgs(eventID, int64(8)).
		WillReturnRows(sqlmock.NewRows(evColNames))
	decode(t, testutil.Request(e.user(8), "POST", "/v1/events/10/listing/quote", nil, nil), 404, nil)
}

// ---------- create order ----------

func (e *env) orderHead(ev evOpt) {
	e.mock.ExpectBegin()
	e.expEvent(uid, true, evRows(ev))
	e.expRules(defaultRule(sqlmock.NewRows(ruleColNames)))
	e.expPolicyDoc(nil)
}

func (e *env) expInsertOrder(provider, status string, total int64, paidAt any, id int64) {
	base, gst, entry := int64(40000), int64(7200), int64(50000)
	if total == 0 {
		base, gst, entry = 0, 0, 0
	}
	e.mock.ExpectExec(`INSERT INTO listing_orders`).
		WithArgs(eventID, uid, int64(1), entry, int64(16), int64(500), int64(1800), base, gst, total, provider, status, paidAt,
			fixedNow.Add(30*time.Minute)).
		WillReturnResult(sqlmock.NewResult(id, 1))
}

func TestCreateOrderMock(t *testing.T) {
	e := newEnv(t, nil)
	e.orderHead(evOpt{status: "draft", fee: 50000})
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.expInsertOrder(providerMock, stCreated, 47200, nil, orderID)
	e.expSetStatus("pending_payment", orderable3...)
	e.mock.ExpectCommit()
	e.expPolicyDoc(nil)
	var o Order
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/order", nil, nil), 200, &o)
	if o.OrderID != orderID || o.Status != stCreated || o.Provider != providerMock || o.EventStatus != "pending_payment" ||
		o.Quote.TotalPaise != 47200 || o.ProviderKeyID != nil || o.ProviderOrderID != nil || !o.ExpiresAt.Equal(fixedNow.Add(30*time.Minute)) {
		t.Fatalf("order %+v", o)
	}
	if len(e.got[events.ListingPaid]) != 0 {
		t.Fatal("listing.paid published before payment")
	}
}

func TestCreateOrderTTLFromSettings(t *testing.T) {
	e := newEnv(t, map[string]any{"listing.order_ttl_minutes": 10})
	e.orderHead(evOpt{status: "rejected", fee: 50000})
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.mock.ExpectExec(`INSERT INTO listing_orders`).
		WithArgs(eventID, uid, int64(1), int64(50000), int64(16), int64(500), int64(1800), int64(40000), int64(7200), int64(47200),
			providerMock, stCreated, nil, fixedNow.Add(10*time.Minute)).
		WillReturnResult(sqlmock.NewResult(orderID, 1))
	e.expSetStatus("pending_payment", orderable3...)
	e.mock.ExpectCommit()
	e.expPolicyDoc(nil)
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/order", nil, nil), 200, nil)
}

func TestCreateOrderReusesOpenOrder(t *testing.T) {
	e := newEnv(t, nil)
	e.orderHead(evOpt{status: "pending_payment", fee: 50000})
	e.expEventOrders(orderRows(stdOrder(providerMock, stCreated)), stCreated, stFailed)
	e.expSetStatus("pending_payment", orderable3...)
	e.mock.ExpectCommit()
	e.expPolicyDoc(nil)
	var o Order
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/order", nil, nil), 200, &o)
	if o.OrderID != orderID || o.Status != stCreated {
		t.Fatalf("order %+v", o)
	}
}

func TestCreateOrderExpiresChangedQuote(t *testing.T) {
	e := newEnv(t, nil)
	e.orderHead(evOpt{status: "pending_payment", fee: 50000})
	old := stdOrder(providerMock, stCreated)
	old.ID, old.EntryFeePaise, old.TotalPaise = 3, 40000, 37760
	failed := stdOrder(providerMock, stFailed)
	failed.ID = 4
	e.expEventOrders(orderRows(old, failed), stCreated, stFailed)
	e.mock.ExpectExec(`UPDATE listing_orders SET status = 'expired' WHERE id IN \(\?,\?\) AND status IN \('created','failed'\)`).
		WithArgs(int64(3), int64(4)).WillReturnResult(sqlmock.NewResult(0, 2))
	e.expInsertOrder(providerMock, stCreated, 47200, nil, 6)
	e.expSetStatus("pending_payment", orderable3...)
	e.mock.ExpectCommit()
	e.expPolicyDoc(nil)
	var o Order
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/order", nil, nil), 200, &o)
	if o.OrderID != 6 {
		t.Fatalf("order %+v", o)
	}
}

func TestCreateOrderZeroTotalIsPaidAtOnce(t *testing.T) {
	e := newEnv(t, nil)
	e.orderHead(evOpt{status: "draft", fee: 0}) // free event, free_event_fee_paise 0
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.expInsertOrder(providerNone, stPaid, 0, fixedNow, orderID)
	e.expSetStatus("pending_review", orderable3...)
	e.mock.ExpectCommit()
	e.expPolicyDoc(nil)
	var o Order
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/order", nil, nil), 200, &o)
	if o.Status != stPaid || o.Provider != providerNone || o.EventStatus != "pending_review" || o.PaidAt == nil {
		t.Fatalf("order %+v", o)
	}
	if p := e.published(events.ListingPaid); len(p) != 1 || p[0].OrderID != orderID || p[0].AmountPaise != 0 {
		t.Fatalf("listing.paid %+v", p)
	}
	if len(e.got[events.EventSubmitted]) != 1 || len(e.got[events.EventPublished]) != 0 {
		t.Fatalf("events %v", e.got)
	}
}

func TestCreateOrderZeroTotalPublishesWithoutApproval(t *testing.T) {
	e := newEnv(t, map[string]any{"event.require_admin_approval": false})
	e.orderHead(evOpt{status: "draft", fee: 0})
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.expInsertOrder(providerNone, stPaid, 0, fixedNow, orderID)
	e.expSetStatus("published", orderable3...)
	e.mock.ExpectCommit()
	e.expPolicyDoc(nil)
	var o Order
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/order", nil, nil), 200, &o)
	if o.EventStatus != "published" || len(e.got[events.EventPublished]) != 1 || len(e.got[events.EventSubmitted]) != 0 {
		t.Fatalf("order %+v events %v", o, e.got)
	}
}

func TestCreateOrderRazorpay(t *testing.T) {
	e := newEnv(t, map[string]any{"listing.payment_provider": "razorpay"})
	e.orderHead(evOpt{status: "draft", fee: 50000})
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.expInsertOrder(providerRazorpay, stCreated, 47200, nil, orderID)
	e.mock.ExpectExec(`UPDATE listing_orders SET provider_order_id = \? WHERE id = \?`).WithArgs("order_fake1", orderID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	e.expSetStatus("pending_payment", orderable3...)
	e.mock.ExpectCommit()
	e.expPolicyDoc(nil)
	var o Order
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/order", nil, nil), 200, &o)
	if o.ProviderOrderID == nil || *o.ProviderOrderID != "order_fake1" || o.ProviderKeyID == nil || *o.ProviderKeyID != "rzp_test_k" {
		t.Fatalf("order %+v", o)
	}
	if len(e.rz.Orders) != 1 || e.rz.Orders[0].Amount != 47200 || e.rz.Orders[0].Receipt != "listing_5" {
		t.Fatalf("razorpay orders %+v", e.rz.Orders)
	}
}

func TestCreateOrderRazorpayFailureRollsBack(t *testing.T) {
	e := newEnv(t, map[string]any{"listing.payment_provider": "razorpay"})
	e.rz.Err = errors.New("timeout")
	e.orderHead(evOpt{status: "draft", fee: 50000})
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.expInsertOrder(providerRazorpay, stCreated, 47200, nil, orderID)
	e.mock.ExpectRollback()
	en := decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/order", nil, nil), 502, nil)
	if strings.Contains(string(en.Data)+en.Error.Code, "timeout") {
		t.Fatal("provider error leaked")
	}
}

func TestCreateOrderMockRefusedInProd(t *testing.T) {
	e := newEnv(t, nil)
	e.d.Cfg.Env = config.EnvProd
	e.orderHead(evOpt{status: "draft", fee: 50000})
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.mock.ExpectRollback()
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/order", nil, nil), 502, nil)
}

func TestCreateOrderWrongStatusIs409(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.expEvent(uid, true, evRows(evOpt{status: "published", fee: 50000}))
	e.mock.ExpectRollback()
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/order", nil, nil), 409, nil)
}

func TestCreateOrderNotOwnerIs404(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(`FROM events e WHERE e.id = \? AND e.organiser_id = \? FOR UPDATE$`).WithArgs(eventID, int64(8)).
		WillReturnRows(sqlmock.NewRows(evColNames))
	e.mock.ExpectRollback()
	decode(t, testutil.Request(e.user(8), "POST", "/v1/events/10/listing/order", nil, nil), 404, nil)
}

func TestCreateOrderSubmitChecks(t *testing.T) {
	e := newEnv(t, map[string]any{"event.min_hours_before_start": 48})
	e.mock.ExpectBegin()
	e.expEvent(uid, true, evRows(evOpt{status: "draft", fee: 50000, starts: fixedNow.Add(30 * time.Hour)}))
	e.mock.ExpectRollback()
	en := decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/order", nil, nil), 422, nil)
	if en.Error.Fields["startsAt"] != "must be at least 48 hours from now" {
		t.Fatalf("fields %v", en.Error.Fields)
	}
}

// ---------- confirm ----------

func sig(orderID, paymentID string) string { return razorpay.Sign([]byte(orderID+"|"+paymentID), "ks") }

func (e *env) confirmHead(o orderRec, ev evOpt) {
	e.mock.ExpectBegin()
	e.expEvent(uid, true, evRows(ev))
	e.mock.ExpectQuery(`WHERE o.id = \? AND o.organiser_id = \? AND o.event_id = \? FOR UPDATE OF o$`).
		WithArgs(orderID, uid, eventID).WillReturnRows(orderRows(o))
}

func TestConfirmHappyPath(t *testing.T) {
	e := newEnv(t, nil)
	e.confirmHead(stdOrder(providerRazorpay, stCreated), evOpt{status: "pending_payment", fee: 50000})
	e.expMarkPaid("pay_abc")
	e.expSetStatus("pending_review", "pending_payment")
	e.mock.ExpectCommit()
	e.expPolicyDoc(nil)
	var o Order
	body := ConfirmInput{OrderID: orderID, ProviderPaymentID: "pay_abc", ProviderSignature: sig("order_x", "pay_abc")}
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/confirm", body, nil), 200, &o)
	if o.Status != stPaid || o.EventStatus != "pending_review" || o.PaidAt == nil {
		t.Fatalf("order %+v", o)
	}
	if p := e.published(events.ListingPaid); len(p) != 1 || p[0].AmountPaise != 47200 || p[0].OrganiserID != uid {
		t.Fatalf("listing.paid %+v", p)
	}
	if len(e.got[events.EventSubmitted]) != 1 {
		t.Fatal("event.submitted not published")
	}
}

func TestConfirmBadSignatureIs422(t *testing.T) {
	e := newEnv(t, nil)
	e.confirmHead(stdOrder(providerRazorpay, stCreated), evOpt{status: "pending_payment", fee: 50000})
	e.mock.ExpectRollback()
	body := ConfirmInput{OrderID: orderID, ProviderPaymentID: "pay_abc", ProviderSignature: sig("order_other", "pay_abc")}
	en := decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/confirm", body, nil), 422, nil)
	if en.Error.Fields["providerSignature"] == "" {
		t.Fatalf("fields %v", en.Error.Fields)
	}
}

func TestConfirmValidation(t *testing.T) {
	e := newEnv(t, nil)
	body := ConfirmInput{ProviderPaymentID: "not a payment"}
	en := decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/confirm", body, nil), 422, nil)
	for _, f := range []string{"orderId", "providerPaymentId", "providerSignature"} {
		if en.Error.Fields[f] == "" {
			t.Errorf("missing %s: %v", f, en.Error.Fields)
		}
	}
}

func TestConfirmIsIdempotent(t *testing.T) {
	e := newEnv(t, nil)
	e.confirmHead(stdOrder(providerRazorpay, stPaid), evOpt{status: "pending_review", fee: 50000})
	e.mock.ExpectCommit()
	e.expPolicyDoc(nil)
	body := ConfirmInput{OrderID: orderID, ProviderPaymentID: "pay_abc", ProviderSignature: "anything"}
	var o Order
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/confirm", body, nil), 200, &o)
	if o.Status != stPaid || len(e.got[events.ListingPaid]) != 0 {
		t.Fatalf("order %+v events %v", o, e.got)
	}
}

func TestConfirmNotOwnerIs404(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.expEvent(uid, true, evRows(evOpt{status: "pending_payment", fee: 50000}))
	e.mock.ExpectQuery(`WHERE o.id = \? AND o.organiser_id = \? AND o.event_id = \? FOR UPDATE OF o$`).
		WithArgs(orderID, uid, eventID).WillReturnRows(sqlmock.NewRows(orderColNames))
	e.mock.ExpectRollback()
	body := ConfirmInput{OrderID: orderID, ProviderPaymentID: "pay_abc", ProviderSignature: sig("order_x", "pay_abc")}
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/confirm", body, nil), 404, nil)
}

func TestConfirmMockOrderIs409(t *testing.T) {
	e := newEnv(t, nil)
	e.confirmHead(stdOrder(providerMock, stCreated), evOpt{status: "pending_payment", fee: 50000})
	e.mock.ExpectRollback()
	body := ConfirmInput{OrderID: orderID, ProviderPaymentID: "pay_abc", ProviderSignature: sig("order_x", "pay_abc")}
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/confirm", body, nil), 409, nil)
}

func TestConfirmLatePaymentIsRefunded(t *testing.T) {
	e := newEnv(t, nil)
	e.confirmHead(stdOrder(providerRazorpay, stExpired), evOpt{status: "draft", fee: 50000})
	e.expMarkPaid("pay_abc")
	e.expAddRefund(47200, stRefunded, "rfnd_fake1", "late_payment", nil, 0)
	e.mock.ExpectCommit()
	body := ConfirmInput{OrderID: orderID, ProviderPaymentID: "pay_abc", ProviderSignature: sig("order_x", "pay_abc")}
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/events/10/listing/confirm", body, nil), 409, nil)
	if p := e.published(events.ListingRefunded); len(p) != 1 || p[0].AmountPaise != 47200 {
		t.Fatalf("listing.refunded %+v", p)
	}
	if len(e.got[events.ListingPaid]) != 0 || len(e.rz.Refunds) != 1 || e.rz.Refunds[0].PaymentID != "pay_abc" {
		t.Fatalf("events %v refunds %+v", e.got, e.rz.Refunds)
	}
}

// ---------- mock pay ----------

func TestMockPayDisabledIs404(t *testing.T) {
	e := newEnv(t, map[string]any{"listing.payment_provider": "razorpay"})
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/listing/mock/pay", MockPayInput{OrderID: orderID}, nil), 404, nil)
	p := newEnv(t, nil)
	p.d.Cfg.Env = config.EnvProd
	decode(t, testutil.Request(p.user(uid), "POST", "/v1/listing/mock/pay", MockPayInput{OrderID: orderID}, nil), 404, nil)
}

func TestMockPayValidation(t *testing.T) {
	e := newEnv(t, nil)
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/listing/mock/pay", MockPayInput{}, nil), 422, nil)
}

func TestMockPayHappyPath(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(`WHERE o.id = \? AND o.organiser_id = \?$`).WithArgs(orderID, uid).
		WillReturnRows(orderRows(stdOrder(providerMock, stCreated)))
	e.expEvent(uid, true, evRows(evOpt{status: "pending_payment", fee: 50000}))
	e.expOrderByID(true, orderRows(stdOrder(providerMock, stCreated)))
	e.expMarkPaid("mock_pay_5")
	e.expSetStatus("pending_review", "pending_payment")
	e.mock.ExpectCommit()
	e.expPolicyDoc(nil)
	var o Order
	decode(t, testutil.Request(e.user(uid), "POST", "/v1/listing/mock/pay", MockPayInput{OrderID: orderID}, nil), 200, &o)
	if o.Status != stPaid || o.EventStatus != "pending_review" || len(e.got[events.ListingPaid]) != 1 {
		t.Fatalf("order %+v", o)
	}
}

func TestMockPayNotOwnerIs404(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(`WHERE o.id = \? AND o.organiser_id = \?$`).WithArgs(orderID, int64(8)).
		WillReturnRows(sqlmock.NewRows(orderColNames))
	e.mock.ExpectRollback()
	decode(t, testutil.Request(e.user(8), "POST", "/v1/listing/mock/pay", MockPayInput{OrderID: orderID}, nil), 404, nil)
}

// ---------- webhook ----------

func webhook(h http.Handler, body, signature, eventID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/v1/webhooks/razorpay", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("X-Razorpay-Signature", signature)
	}
	if eventID != "" {
		req.Header.Set("X-Razorpay-Event-Id", eventID)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

const capturedBody = `{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_abc","order_id":"order_x","amount":47200,"status":"captured"}}}}`

func (e *env) expProviderLookup(found bool) {
	rows := sqlmock.NewRows([]string{"id", "event_id"})
	if found {
		rows.AddRow(orderID, eventID)
	}
	e.mock.ExpectQuery(`SELECT id, event_id FROM listing_orders WHERE provider = 'razorpay' AND provider_order_id = \?`).
		WithArgs("order_x").WillReturnRows(rows)
}

func (e *env) expRecord(evID, typ, entity, status string, dup bool) {
	ex := e.mock.ExpectExec(`INSERT INTO payment_webhook_events`).WithArgs(evID, typ, entity, status, fixedNow, fixedNow)
	if dup {
		ex.WillReturnError(errors.New("Error 1062: Duplicate entry 'razorpay-evt_1'"))
	} else {
		ex.WillReturnResult(sqlmock.NewResult(1, 1))
	}
}

func TestWebhookBadSignatureIs401WithoutSQL(t *testing.T) {
	e := newEnv(t, nil)
	h := e.user(uid)
	decode(t, webhook(h, capturedBody, "", "evt_1"), 401, nil)
	decode(t, webhook(h, capturedBody, razorpay.Sign([]byte(capturedBody), "wrong"), "evt_1"), 401, nil)
}

func TestWebhookCapturedThenDuplicate(t *testing.T) {
	e := newEnv(t, nil)
	h := e.user(uid)
	s := razorpay.Sign([]byte(capturedBody), "ws")

	e.mock.ExpectBegin()
	e.expProviderLookup(true)
	e.expRecord("evt_1", "payment.captured", "pay_abc", "processed", false)
	e.expEvent(0, true, evRows(evOpt{status: "pending_payment", fee: 50000}))
	e.expOrderByID(true, orderRows(stdOrder(providerRazorpay, stCreated)))
	e.expMarkPaid("pay_abc")
	e.expSetStatus("pending_review", "pending_payment")
	e.mock.ExpectCommit()
	decode(t, webhook(h, capturedBody, s, "evt_1"), 200, nil)

	// Re-delivery of the same event: dedupe row exists, nothing else runs.
	e.mock.ExpectBegin()
	e.expProviderLookup(true)
	e.expRecord("evt_1", "payment.captured", "pay_abc", "processed", true)
	e.mock.ExpectCommit()
	decode(t, webhook(h, capturedBody, s, "evt_1"), 200, nil)

	if len(e.got[events.ListingPaid]) != 1 || len(e.got[events.EventSubmitted]) != 1 {
		t.Fatalf("events published %v", e.got)
	}
}

func TestWebhookAlreadyConfirmedIsNoop(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.expProviderLookup(true)
	e.expRecord("evt_2", "payment.captured", "pay_abc", "processed", false)
	e.expEvent(0, true, evRows(evOpt{status: "pending_review", fee: 50000}))
	e.expOrderByID(true, orderRows(stdOrder(providerRazorpay, stPaid)))
	e.mock.ExpectCommit()
	decode(t, webhook(e.user(uid), capturedBody, razorpay.Sign([]byte(capturedBody), "ws"), "evt_2"), 200, nil)
	if len(e.got[events.ListingPaid]) != 0 {
		t.Fatal("paid twice")
	}
}

func TestWebhookUnknownOrderIgnoredAndEventIDFromBody(t *testing.T) {
	e := newEnv(t, nil)
	sum := sha256.Sum256([]byte(capturedBody))
	e.mock.ExpectBegin()
	e.expProviderLookup(false)
	e.expRecord(hex.EncodeToString(sum[:]), "payment.captured", "pay_abc", "ignored", false)
	e.mock.ExpectCommit()
	decode(t, webhook(e.user(uid), capturedBody, razorpay.Sign([]byte(capturedBody), "ws"), ""), 200, nil)
}

func TestWebhookAmountMismatchDoesNotSettle(t *testing.T) {
	e := newEnv(t, nil)
	body := strings.Replace(capturedBody, "47200", "100", 1)
	e.mock.ExpectBegin()
	e.expProviderLookup(true)
	e.expRecord("evt_3", "payment.captured", "pay_abc", "processed", false)
	e.expEvent(0, true, evRows(evOpt{status: "pending_payment", fee: 50000}))
	e.expOrderByID(true, orderRows(stdOrder(providerRazorpay, stCreated)))
	e.mock.ExpectCommit()
	decode(t, webhook(e.user(uid), body, razorpay.Sign([]byte(body), "ws"), "evt_3"), 200, nil)
}

func TestWebhookPaymentFailed(t *testing.T) {
	e := newEnv(t, nil)
	body := `{"event":"payment.failed","payload":{"payment":{"entity":{"id":"pay_abc","order_id":"order_x","amount":47200,"status":"failed"}}}}`
	e.mock.ExpectBegin()
	e.expProviderLookup(true)
	e.expRecord("evt_4", "payment.failed", "pay_abc", "processed", false)
	e.expEvent(0, true, evRows(evOpt{status: "pending_payment", fee: 50000}))
	e.expOrderByID(true, orderRows(stdOrder(providerRazorpay, stCreated)))
	e.mock.ExpectExec(`UPDATE listing_orders SET status = 'failed' WHERE id = \? AND status = 'created'`).WithArgs(orderID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	decode(t, webhook(e.user(uid), body, razorpay.Sign([]byte(body), "ws"), "evt_4"), 200, nil)
}

func TestWebhookOrderPaid(t *testing.T) {
	e := newEnv(t, nil)
	body := `{"event":"order.paid","payload":{"order":{"entity":{"id":"order_x","amount":47200}},"payment":{"entity":{"id":"pay_abc","order_id":"order_x","amount":47200}}}}`
	e.mock.ExpectBegin()
	e.expProviderLookup(true)
	e.expRecord("evt_5", "order.paid", "order_x", "processed", false)
	e.expEvent(0, true, evRows(evOpt{status: "pending_payment", fee: 50000}))
	e.expOrderByID(true, orderRows(stdOrder(providerRazorpay, stCreated)))
	e.expMarkPaid("pay_abc")
	e.expSetStatus("pending_review", "pending_payment")
	e.mock.ExpectCommit()
	decode(t, webhook(e.user(uid), body, razorpay.Sign([]byte(body), "ws"), "evt_5"), 200, nil)
	if len(e.got[events.ListingPaid]) != 1 {
		t.Fatal("listing.paid not published")
	}
}

func TestWebhookRefundProcessed(t *testing.T) {
	e := newEnv(t, nil)
	body := `{"event":"refund.processed","payload":{"refund":{"entity":{"id":"rfnd_1","payment_id":"pay_abc","amount":47200,"status":"processed"}}}}`
	h := e.user(uid)
	s := razorpay.Sign([]byte(body), "ws")
	e.mock.ExpectBegin()
	e.expRecord("evt_6", "refund.processed", "rfnd_1", "processed", false)
	e.mock.ExpectExec(`UPDATE listing_orders SET refunded_at = COALESCE\(refunded_at, \?\) WHERE provider = 'razorpay' AND provider_refund_id = \?`).
		WithArgs(fixedNow, "rfnd_1").WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	decode(t, webhook(h, body, s, "evt_6"), 200, nil)
	e.mock.ExpectBegin()
	e.expRecord("evt_6", "refund.processed", "rfnd_1", "processed", true)
	e.mock.ExpectCommit()
	decode(t, webhook(h, body, s, "evt_6"), 200, nil)
}

// ---------- event handlers ----------

func cancelled(typ string) events.Event {
	return events.New(typ, 0, events.EventPayload{EventID: eventID, OrganiserID: uid, SportID: 4})
}

func TestOnCancelledBeforeRegistrationsRefundsFullAndIsIdempotent(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	e.mock.ExpectBegin()
	e.expEvent(0, true, evRows(evOpt{status: "cancelled", fee: 50000}))
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.expEventOrders(orderRows(stdOrder(providerMock, stPaid)), stPaid)
	e.expAddRefund(47200, stRefunded, nil, "event_cancelled", fixedNow, 0)
	e.mock.ExpectCommit()
	if err := e.m.svc.OnCancelled(ctx, cancelled(events.EventCancelled)); err != nil {
		t.Fatal(err)
	}
	// Re-delivery: the order is no longer `paid`, so nothing is refunded again.
	e.mock.ExpectBegin()
	e.expEvent(0, true, evRows(evOpt{status: "cancelled", fee: 50000}))
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.expEventOrders(sqlmock.NewRows(orderColNames), stPaid)
	e.mock.ExpectCommit()
	if err := e.m.svc.OnCancelled(ctx, cancelled(events.EventCancelled)); err != nil {
		t.Fatal(err)
	}
	if p := e.published(events.ListingRefunded); len(p) != 1 || p[0].AmountPaise != 47200 {
		t.Fatalf("listing.refunded %+v", p)
	}
}

func TestOnCancelledAfterRegistrationsUsesSetting(t *testing.T) {
	e := newEnv(t, nil) // default after_registrations_percent = 0
	e.mock.ExpectBegin()
	e.expEvent(0, true, evRows(evOpt{status: "cancelled", fee: 50000, regs: 3}))
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.mock.ExpectCommit()
	if err := e.m.svc.OnCancelled(context.Background(), cancelled(events.EventCancelled)); err != nil {
		t.Fatal(err)
	}
	if len(e.got[events.ListingRefunded]) != 0 {
		t.Fatal("refunded with 0%")
	}

	h := newEnv(t, map[string]any{"listing.refund.after_registrations_percent": 50})
	h.mock.ExpectBegin()
	h.expEvent(0, true, evRows(evOpt{status: "cancelled", fee: 50000, regs: 3}))
	h.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	h.expEventOrders(orderRows(stdOrder(providerMock, stPaid)), stPaid)
	h.expAddRefund(23600, stPartially, nil, "event_cancelled", fixedNow, 0)
	h.mock.ExpectCommit()
	if err := h.m.svc.OnCancelled(context.Background(), cancelled(events.EventCancelled)); err != nil {
		t.Fatal(err)
	}
	if p := h.published(events.ListingRefunded); len(p) != 1 || p[0].AmountPaise != 23600 {
		t.Fatalf("listing.refunded %+v", p)
	}
}

func TestOnCancelledRemovedExpiresOnly(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.expEvent(0, true, evRows(evOpt{status: "removed", fee: 50000}))
	e.expEventOrders(orderRows(stdOrder(providerMock, stCreated)), stCreated, stFailed)
	e.mock.ExpectExec(`UPDATE listing_orders SET status = 'expired' WHERE id IN \(\?\)`).WithArgs(orderID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	if err := e.m.svc.OnCancelled(context.Background(), cancelled(events.EventCancelled)); err != nil {
		t.Fatal(err)
	}
	if len(e.got[events.ListingRefunded]) != 0 {
		t.Fatal("removed event refunded")
	}
}

func TestOnCancelledMissingEventAndBadPayload(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.expEvent(0, true, sqlmock.NewRows(evColNames))
	e.mock.ExpectCommit()
	if err := e.m.svc.OnCancelled(context.Background(), cancelled(events.EventCancelled)); err != nil {
		t.Fatal(err)
	}
	if err := e.m.svc.OnCancelled(context.Background(), events.Event{Type: events.EventCancelled, Payload: json.RawMessage(`"x"`)}); err != nil {
		t.Fatal(err)
	}
}

func TestOnRejectedRefundsThroughRazorpay(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.expEvent(0, true, evRows(evOpt{status: "rejected", fee: 50000}))
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.expEventOrders(orderRows(stdOrder(providerRazorpay, stPaid)), stPaid)
	e.expAddRefund(47200, stRefunded, "rfnd_fake1", "admin_rejected", nil, 0) // refund pending at Razorpay
	e.mock.ExpectCommit()
	if err := e.m.svc.OnRejected(context.Background(), cancelled(events.EventRejected)); err != nil {
		t.Fatal(err)
	}
	if len(e.rz.Refunds) != 1 || e.rz.Refunds[0].Amount != 47200 || e.rz.Refunds[0].PaymentID != "pay_abc" {
		t.Fatalf("refunds %+v", e.rz.Refunds)
	}
}

func TestOnRejectedSettingAndResubmitted(t *testing.T) {
	e := newEnv(t, map[string]any{"listing.refund.on_admin_rejection_percent": 0})
	e.mock.ExpectBegin()
	e.expEvent(0, true, evRows(evOpt{status: "rejected", fee: 50000}))
	e.expEventOrders(sqlmock.NewRows(orderColNames), stCreated, stFailed)
	e.mock.ExpectCommit()
	if err := e.m.svc.OnRejected(context.Background(), cancelled(events.EventRejected)); err != nil {
		t.Fatal(err)
	}
	// The organiser already resubmitted: nothing happens.
	e.mock.ExpectBegin()
	e.expEvent(0, true, evRows(evOpt{status: "pending_payment", fee: 50000}))
	e.mock.ExpectCommit()
	if err := e.m.svc.OnRejected(context.Background(), cancelled(events.EventRejected)); err != nil {
		t.Fatal(err)
	}
	if len(e.got[events.ListingRefunded]) != 0 {
		t.Fatal("refunded")
	}
}

func TestOnUpdated(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.expEvent(0, true, evRows(evOpt{status: "draft", fee: 50000}))
	e.expEventOrders(orderRows(stdOrder(providerMock, stCreated)), stCreated, stFailed)
	e.mock.ExpectExec(`UPDATE listing_orders SET status = 'expired' WHERE id IN \(\?\)`).WithArgs(orderID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	if err := e.m.svc.OnUpdated(context.Background(), cancelled(events.EventUpdated)); err != nil {
		t.Fatal(err)
	}
	e.mock.ExpectBegin()
	e.expEvent(0, true, evRows(evOpt{status: "pending_payment", fee: 50000}))
	e.mock.ExpectCommit()
	if err := e.m.svc.OnUpdated(context.Background(), cancelled(events.EventUpdated)); err != nil {
		t.Fatal(err)
	}
}

// ---------- job ----------

func TestExpireOrdersJob(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(`SELECT id, event_id FROM listing_orders WHERE status IN \('created','failed'\) AND expires_at <= \? ORDER BY id ASC LIMIT \? FOR UPDATE`).
		WithArgs(fixedNow, batchSize).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_id"}).AddRow(3, 10).AddRow(4, 10).AddRow(6, 11))
	e.mock.ExpectExec(`UPDATE listing_orders SET status = 'expired' WHERE id IN \(\?,\?,\?\)`).
		WithArgs(int64(3), int64(4), int64(6)).WillReturnResult(sqlmock.NewResult(0, 3))
	for _, ev := range []int64{10, 11} {
		e.mock.ExpectExec(`UPDATE events SET status = 'draft' WHERE id = \? AND status = 'pending_payment' AND NOT EXISTS`).
			WithArgs(ev, ev, fixedNow).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	e.mock.ExpectCommit()
	if err := e.m.svc.ExpireOrders(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Re-run with nothing due.
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(`FROM listing_orders WHERE status IN`).WillReturnRows(sqlmock.NewRows([]string{"id", "event_id"}))
	e.mock.ExpectCommit()
	if err := e.m.svc.ExpireOrders(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestModuleWiring(t *testing.T) {
	m := newEnv(t, nil).m
	if m.Name() != "listingpayment" || m.Jobs()["listing.expire_orders"] == nil {
		t.Fatal("job missing")
	}
	h := m.EventHandlers()
	for _, typ := range []string{events.EventCancelled, events.EventRejected, events.EventUpdated} {
		if h[typ] == nil {
			t.Errorf("handler %s missing", typ)
		}
	}
}

// ---------- admin ----------

func TestAdminRoutesNeedPermission(t *testing.T) {
	e := newEnv(t, nil)
	h := e.admin("users.read")
	for _, r := range [][2]string{
		{"GET", "/v1/admin/listing-orders"},
		{"POST", "/v1/admin/listing-orders/5/refund"},
		{"GET", "/v1/admin/fee-rules"},
		{"POST", "/v1/admin/fee-rules"},
		{"PUT", "/v1/admin/fee-rules/1"},
		{"POST", "/v1/admin/fee-rules/preview"},
	} {
		if w := testutil.Request(h, r[0], r[1], map[string]any{}, nil); w.Code != 403 {
			t.Errorf("%s %s = %d", r[0], r[1], w.Code)
		}
	}
}

func TestAdminOrdersList(t *testing.T) {
	e := newEnv(t, nil)
	a, b := stdOrder(providerRazorpay, stPaid), stdOrder(providerRazorpay, stPaid)
	b.ID = 4
	e.mock.ExpectQuery(`WHERE 1=1 AND o.status = \? AND o.event_id = \? AND o.created_at >= \? ORDER BY o.id DESC LIMIT \?`).
		WithArgs(stPaid, eventID, time.Date(2026, 10, 31, 18, 30, 0, 0, time.UTC), 2).WillReturnRows(orderRows(a, b))
	e.expPolicyDoc(nil)
	e.mock.ExpectQuery(`FROM users u\s+LEFT JOIN profiles`).WithArgs(uid).
		WillReturnRows(sqlmock.NewRows([]string{"id", "dn", "pr", "vs", "photo", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"}).
			AddRow(uid, "Organiser", "organiser", "verified", nil, nil, nil, nil, nil, nil, nil, nil, nil))
	var out []AdminOrder
	en := decode(t, testutil.Request(e.admin("payments.read"), "GET", "/v1/admin/listing-orders?status=paid&eventId=10&from=2026-11-01&limit=1", nil, nil), 200, &out)
	if len(out) != 1 || out[0].OrderID != orderID || out[0].Organiser == nil || out[0].Organiser.ID != uid ||
		out[0].ProviderKeyID == nil || en.Meta["nextCursor"] == "" || en.Meta["nextCursor"] == nil {
		t.Fatalf("out %+v meta %v", out, en.Meta)
	}
}

func TestAdminOrdersValidation(t *testing.T) {
	e := newEnv(t, nil)
	h := e.admin("payments.read")
	decode(t, testutil.Request(h, "GET", "/v1/admin/listing-orders?status=lost", nil, nil), 422, nil)
	decode(t, testutil.Request(h, "GET", "/v1/admin/listing-orders?from=soon", nil, nil), 422, nil)
}

func (e *env) refundHead(o orderRec) {
	e.mock.ExpectBegin()
	e.expOrderByID(false, orderRows(o))
	e.expEvent(0, true, evRows(evOpt{status: "published", fee: 50000}))
	e.expOrderByID(true, orderRows(o))
}

func TestAdminRefund(t *testing.T) {
	e := newEnv(t, nil)
	e.refundHead(stdOrder(providerMock, stPaid))
	e.expAddRefund(10000, stPartially, nil, "Duplicate listing", fixedNow, 0)
	e.mock.ExpectExec(`INSERT INTO admin_audit_log`).
		WithArgs(int64(9), "listing.refund", "listing_order", "5", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectCommit()
	e.expPolicyDoc(nil)
	var o Order
	decode(t, testutil.Request(e.admin("payments.refund"), "POST", "/v1/admin/listing-orders/5/refund",
		RefundInput{AmountPaise: 10000, Reason: " Duplicate listing "}, nil), 200, &o)
	if o.Status != stPartially || o.RefundPaise != 10000 {
		t.Fatalf("order %+v", o)
	}
	evs := e.got[events.ListingRefunded]
	if len(evs) != 1 || evs[0].ActorID != 9 || e.published(events.ListingRefunded)[0].AmountPaise != 10000 {
		t.Fatalf("listing.refunded %+v", evs)
	}
}

func TestAdminRefundOverRemainingIs422(t *testing.T) {
	e := newEnv(t, nil)
	o := stdOrder(providerMock, stPartially)
	o.RefundPaise = 40000
	e.refundHead(o)
	e.mock.ExpectRollback()
	en := decode(t, testutil.Request(e.admin("payments.refund"), "POST", "/v1/admin/listing-orders/5/refund",
		RefundInput{AmountPaise: 10000, Reason: "Goodwill"}, nil), 422, nil)
	if en.Error.Fields["amountPaise"] != "must be at most 7200" {
		t.Fatalf("fields %v", en.Error.Fields)
	}
}

func TestAdminRefundNotPaidIs409(t *testing.T) {
	e := newEnv(t, nil)
	e.refundHead(stdOrder(providerMock, stCreated))
	e.mock.ExpectRollback()
	decode(t, testutil.Request(e.admin("payments.refund"), "POST", "/v1/admin/listing-orders/5/refund",
		RefundInput{AmountPaise: 100, Reason: "Goodwill"}, nil), 409, nil)
}

func TestAdminRefundValidationAnd404(t *testing.T) {
	e := newEnv(t, nil)
	h := e.admin("payments.refund")
	en := decode(t, testutil.Request(h, "POST", "/v1/admin/listing-orders/5/refund", RefundInput{AmountPaise: 0, Reason: " "}, nil), 422, nil)
	if en.Error.Fields["amountPaise"] == "" || en.Error.Fields["reason"] == "" {
		t.Fatalf("fields %v", en.Error.Fields)
	}
	e.mock.ExpectBegin()
	e.expOrderByID(false, sqlmock.NewRows(orderColNames))
	e.mock.ExpectRollback()
	decode(t, testutil.Request(h, "POST", "/v1/admin/listing-orders/5/refund", RefundInput{AmountPaise: 100, Reason: "Goodwill"}, nil), 404, nil)
}

func TestFeeRulesList(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectQuery(`FROM listing_fee_rules ORDER BY active DESC, priority DESC, id DESC`).
		WillReturnRows(sportRule(defaultRule(sqlmock.NewRows(ruleColNames))))
	var out []FeeRule
	decode(t, testutil.Request(e.admin("config.read"), "GET", "/v1/admin/fee-rules", nil, nil), 200, &out)
	if len(out) != 2 || out[1].SportID == nil || *out[1].SportID != 4 || out[0].MaxFeePaise == nil {
		t.Fatalf("rules %+v", out)
	}
}

func TestCreateFeeRule(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.mock.ExpectExec(`INSERT INTO listing_fee_rules`).
		WithArgs("Cricket", int64(4), nil, int64(400), int64(9900), nil, int64(0), int64(1800), int64(0), true,
			fixedNow.Truncate(time.Millisecond), nil, int64(9)).
		WillReturnResult(sqlmock.NewResult(12, 1))
	e.mock.ExpectExec(`INSERT INTO config_audit`).WithArgs("listing_fee_rules", "12", nil, sqlmock.AnyArg(), int64(9)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectExec(`INSERT INTO admin_audit_log`).
		WithArgs(int64(9), "fee_rule.create", "listing_fee_rule", "12", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectCommit()
	body := map[string]any{"name": " Cricket ", "sportId": 4, "percentBp": 400, "minFeePaise": 9900, "maxFeePaise": nil, "freeEventFeePaise": 0}
	var r FeeRule
	decode(t, testutil.Request(e.admin("fees.write"), "POST", "/v1/admin/fee-rules", body, nil), 201, &r)
	if r.ID != 12 || r.Name != "Cricket" || r.GstBp != 1800 || !r.Active || r.MaxFeePaise != nil {
		t.Fatalf("rule %+v", r)
	}
}

func TestCreateFeeRuleValidation(t *testing.T) {
	e := newEnv(t, nil)
	h := e.admin("fees.write")
	en := decode(t, testutil.Request(h, "POST", "/v1/admin/fee-rules", map[string]any{"percentBp": 20000, "eventType": "party"}, nil), 422, nil)
	for _, f := range []string{"name", "percentBp", "minFeePaise", "freeEventFeePaise", "eventType"} {
		if en.Error.Fields[f] == "" {
			t.Errorf("missing %s: %v", f, en.Error.Fields)
		}
	}
	body := map[string]any{"name": "X", "percentBp": 100, "minFeePaise": 5000, "maxFeePaise": 1000, "freeEventFeePaise": 0,
		"effectiveFrom": "2026-12-01T00:00:00Z", "effectiveTo": "2026-11-01T00:00:00Z"}
	en = decode(t, testutil.Request(h, "POST", "/v1/admin/fee-rules", body, nil), 422, nil)
	if en.Error.Fields["maxFeePaise"] == "" || en.Error.Fields["effectiveTo"] == "" {
		t.Fatalf("fields %v", en.Error.Fields)
	}
}

func (e *env) expRuleLock(rows *sqlmock.Rows) {
	e.mock.ExpectQuery(`FROM listing_fee_rules WHERE id = \? FOR UPDATE`).WithArgs(int64(1)).WillReturnRows(rows)
}

func (e *env) expOtherDefaults(n int) {
	e.mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(SELECT id FROM listing_fee_rules WHERE active = 1 AND sport_id IS NULL AND event_type IS NULL AND id <> \? FOR UPDATE\) t`).
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func TestUpdateFeeRuleKeepsOneDefault(t *testing.T) {
	e := newEnv(t, nil)
	h := e.admin("fees.write")
	e.mock.ExpectBegin()
	e.expRuleLock(defaultRule(sqlmock.NewRows(ruleColNames)))
	e.expOtherDefaults(0)
	e.mock.ExpectRollback()
	decode(t, testutil.Request(h, "PUT", "/v1/admin/fee-rules/1", map[string]any{"active": false}, nil), 409, nil)

	e.mock.ExpectBegin()
	e.expRuleLock(defaultRule(sqlmock.NewRows(ruleColNames)))
	e.expOtherDefaults(1)
	e.mock.ExpectExec(`UPDATE listing_fee_rules SET name = \?`).
		WithArgs("Default", nil, nil, int64(500), int64(9900), int64(499900), int64(0), int64(1800), int64(0), false,
			fixedNow.Add(-24*time.Hour), nil, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`INSERT INTO config_audit`).WithArgs("listing_fee_rules", "1", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(9)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectExec(`INSERT INTO admin_audit_log`).
		WithArgs(int64(9), "fee_rule.update", "listing_fee_rule", "1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectCommit()
	var r FeeRule
	decode(t, testutil.Request(h, "PUT", "/v1/admin/fee-rules/1", map[string]any{"active": false}, nil), 200, &r)
	if r.Active || r.PercentBp != 500 {
		t.Fatalf("rule %+v", r)
	}
}

func TestUpdateFeeRulePartialNoDefaultCheck(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectBegin()
	e.expRuleLock(defaultRule(sqlmock.NewRows(ruleColNames)))
	e.mock.ExpectExec(`UPDATE listing_fee_rules SET name = \?`).
		WithArgs("Default", nil, nil, int64(600), int64(9900), nil, int64(0), int64(1800), int64(0), true,
			fixedNow.Add(-24*time.Hour), nil, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`INSERT INTO config_audit`).WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectExec(`INSERT INTO admin_audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectCommit()
	decode(t, testutil.Request(e.admin("fees.write"), "PUT", "/v1/admin/fee-rules/1", map[string]any{"percentBp": 600, "maxFeePaise": nil}, nil), 200, nil)
}

func TestUpdateFeeRuleNotFoundAndInvalid(t *testing.T) {
	e := newEnv(t, nil)
	h := e.admin("fees.write")
	e.mock.ExpectBegin()
	e.expRuleLock(sqlmock.NewRows(ruleColNames))
	e.mock.ExpectRollback()
	decode(t, testutil.Request(h, "PUT", "/v1/admin/fee-rules/1", map[string]any{"active": false}, nil), 404, nil)
	e.mock.ExpectBegin()
	e.expRuleLock(defaultRule(sqlmock.NewRows(ruleColNames)))
	e.mock.ExpectRollback()
	decode(t, testutil.Request(h, "PUT", "/v1/admin/fee-rules/1", map[string]any{"gstBp": -1}, nil), 422, nil)
}

func TestPreview(t *testing.T) {
	e := newEnv(t, map[string]any{"event.allow_free_events": false}) // preview ignores the free-event switch
	e.expRules(sportRule(defaultRule(sqlmock.NewRows(ruleColNames))))
	e.expPolicyDoc(nil)
	var q Quote
	body := QuoteInput{SportID: 4, EventType: "tournament", EntryFeePaise: 0, MaxSlots: 16}
	decode(t, testutil.Request(e.admin("config.read"), "POST", "/v1/admin/fee-rules/preview", body, nil), 200, &q)
	if q.FeeRuleID != 2 || !q.IsFreeEvent || q.TotalPaise != 0 {
		t.Fatalf("quote %+v", q)
	}
}

func TestLatestOrder(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectQuery(`WHERE o.event_id = \? AND o.organiser_id = \? ORDER BY o.id DESC LIMIT 1$`).
		WithArgs(eventID, uid).WillReturnRows(orderRows(stdOrder(providerRazorpay, stPaid)))
	e.expPolicyDoc(nil)
	var o Order
	decode(t, testutil.Request(e.user(uid), "GET", "/v1/events/10/listing/order", nil, nil), 200, &o)
	if o.OrderID != orderID || o.Status != stPaid || o.EventStatus != "pending_payment" {
		t.Fatalf("order %+v", o)
	}
}

func TestLatestOrderNotOwnerIs404(t *testing.T) {
	e := newEnv(t, nil)
	e.mock.ExpectQuery(`WHERE o.event_id = \? AND o.organiser_id = \? ORDER BY o.id DESC LIMIT 1$`).
		WithArgs(eventID, int64(99)).WillReturnRows(sqlmock.NewRows(orderColNames))
	decode(t, testutil.Request(e.user(99), "GET", "/v1/events/10/listing/order", nil, nil), 404, nil)
}

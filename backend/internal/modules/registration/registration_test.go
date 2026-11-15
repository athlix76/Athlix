package registration

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/testutil"
)

const (
	playerID int64 = 1
	orgID    int64 = 2
	otherID  int64 = 3
	evID     int64 = 9
	regID    int64 = 31

	thanjavur  int64 = 26
	coimbatore int64 = 4
	ernakulam  int64 = 9001
	tamilNadu  int64 = 23
)

var (
	t0     = time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)
	starts = t0.Add(10 * 24 * time.Hour)
)

// ---------- helpers ----------

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Meta  map[string]any  `json:"meta"`
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func decode(t *testing.T, b []byte) envelope {
	t.Helper()
	var e envelope
	if len(b) == 0 {
		return e
	}
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatalf("bad json %s", b)
	}
	return e
}

func deps(t *testing.T, cfg map[string]any) (*app.Deps, sqlmock.Sqlmock) {
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return t0 }
	return d, mock
}

func engine(d *app.Deps, uid int64) http.Handler {
	r, routes := testutil.UserEngine(testutil.VerifiedUser(uid))
	New(d).Routes(routes)
	return r
}

func capture(d *app.Deps) *[]events.Event {
	var got []events.Event
	for _, typ := range []string{events.RegistrationCreated, events.RegistrationWithdrawn, events.RegistrationPromoted,
		events.RegistrationPaymentMark, events.RegistrationEligibilityFlagged} {
		d.Dispatcher.On(typ, func(_ context.Context, e events.Event) error { got = append(got, e); return nil })
	}
	return &got
}

func types(evs []events.Event) []string {
	out := []string{}
	for _, e := range evs {
		out = append(out, e.Type)
	}
	return out
}

func payload(t *testing.T, e events.Event) events.RegistrationPayload {
	t.Helper()
	var p events.RegistrationPayload
	if err := e.Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

type row map[string]driver.Value

var evColNames = strings.Split("id,organiser_id,status,starts_at,registration_deadline,max_slots,registrations_count,"+
	"gender_category,age_min,age_max,eligibility_type,eligibility_scope,eligibility_district_id,"+
	"eligibility_state_id,require_verified", ",")

func evRow(status string) row {
	return row{"id": evID, "organiser_id": orgID, "status": status, "starts_at": starts,
		"registration_deadline": starts.Add(-24 * time.Hour), "max_slots": int64(16), "registrations_count": int64(3),
		"gender_category": "open", "eligibility_type": "open", "require_verified": true}
}

func (r row) rule(typ, scope string, district, state int64, verified bool) row {
	r["eligibility_type"], r["eligibility_scope"], r["require_verified"] = typ, scope, verified
	if district > 0 {
		r["eligibility_district_id"] = district
	}
	if state > 0 {
		r["eligibility_state_id"] = state
	}
	return r
}

func vals(cols []string, r row) []driver.Value {
	out := make([]driver.Value, 0, len(cols))
	for _, c := range cols {
		out = append(out, r[c])
	}
	return out
}

func expectEvent(mock sqlmock.Sqlmock, r row, forUpdate bool) {
	re := `FROM events e WHERE e.id = \?$`
	if forUpdate {
		re = `FROM events e WHERE e.id = \? FOR UPDATE$`
	}
	mock.ExpectQuery(re).WithArgs(evID).WillReturnRows(sqlmock.NewRows(evColNames).AddRow(vals(evColNames, r)...))
}

func expectBlocked(mock sqlmock.Sqlmock, a, b int64, n int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blocks`).WithArgs(a, b, b, a).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func expectFacts(mock sqlmock.Sqlmock, uid int64, birthYear driver.Value, gender driver.Value) {
	by := sqlmock.NewRows([]string{"b"})
	if birthYear != nil {
		by.AddRow(birthYear)
	}
	mock.ExpectQuery(`SELECT birth_year FROM profiles WHERE user_id = \?`).WithArgs(uid).WillReturnRows(by)
	g := sqlmock.NewRows([]string{"g"})
	if gender != nil {
		g.AddRow(gender)
	}
	mock.ExpectQuery(`SELECT gender FROM identity_verifications WHERE user_id = \? AND status = 'verified'`).WithArgs(uid).WillReturnRows(g)
}

type ident struct {
	id       int64
	typ      string
	district int64
	state    int64
	status   string
	expires  *time.Time
}

func expectIdentities(mock sqlmock.Sqlmock, uid int64, ids ...ident) {
	rows := sqlmock.NewRows([]string{"id", "t", "d", "s", "st", "exp"})
	for _, i := range ids {
		var exp driver.Value
		if i.expires != nil {
			exp = *i.expires
		}
		rows.AddRow(i.id, i.typ, i.district, i.state, i.status, exp)
	}
	mock.ExpectQuery(`FROM user_location_identities WHERE user_id = \? AND status IN`).WithArgs(uid).WillReturnRows(rows)
}

var regColNames = strings.Split("id,event_id,user_id,team_name,team_members,status,payment_status,payment_note,"+
	"created_at,qualified_via,qualified_identity_id,qualified_district_id,eligibility_flag,eligibility_flagged_at", ",")

func regRow(id, uid int64, status string) row {
	return row{"id": id, "event_id": evID, "user_id": uid, "team_members": []byte(`[]`), "status": status,
		"payment_status": "unpaid", "created_at": t0, "qualified_via": "open", "eligibility_flag": "none"}
}

func (r row) via(typ string, identity, district int64) row {
	r["qualified_via"], r["qualified_identity_id"], r["qualified_district_id"] = typ, identity, district
	return r
}

func regRows(rs ...row) *sqlmock.Rows {
	out := sqlmock.NewRows(regColNames)
	for _, r := range rs {
		out.AddRow(vals(regColNames, r)...)
	}
	return out
}

func expectMine(mock sqlmock.Sqlmock, uid int64, rs ...row) {
	mock.ExpectQuery(`FROM event_registrations r WHERE r.event_id = \? AND r.user_id = \? FOR UPDATE`).
		WithArgs(evID, uid).WillReturnRows(regRows(rs...))
}

func expectInsert(mock sqlmock.Sqlmock, uid int64, status, via string, identity, district driver.Value) {
	mock.ExpectExec(`INSERT INTO event_registrations`).
		WithArgs(evID, uid, sqlmock.AnyArg(), sqlmock.AnyArg(), status, via, identity, district).
		WillReturnResult(sqlmock.NewResult(regID, 1))
}

func expectBump(mock sqlmock.Sqlmock, delta int) {
	mock.ExpectExec(`UPDATE events SET registrations_count = GREATEST\(registrations_count \+ \?, 0\) WHERE id = \?`).
		WithArgs(delta, evID).WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectSummaries(mock sqlmock.Sqlmock, ids ...int64) {
	rows := sqlmock.NewRows([]string{"id", "dn", "pr", "vs", "photo", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"})
	args := []driver.Value{}
	for _, id := range ids {
		rows.AddRow(id, "Player", "player", "verified", nil, nil, nil, nil, nil, nil, nil, nil, nil)
		args = append(args, id)
	}
	mock.ExpectQuery(`FROM users u\s+LEFT JOIN profiles`).WithArgs(args...).WillReturnRows(rows)
}

// registerHead is the start of every POST that reaches the eligibility step.
func registerHead(mock sqlmock.Sqlmock, ev row) {
	mock.ExpectBegin()
	expectEvent(mock, ev, true)
	expectBlocked(mock, playerID, orgID, 0)
	expectFacts(mock, playerID, nil, nil)
}

func post(t *testing.T, d *app.Deps, body any) (int, envelope) {
	t.Helper()
	w := testutil.Request(engine(d, playerID), http.MethodPost, "/v1/events/9/registrations", body, nil)
	return w.Code, decode(t, w.Body.Bytes())
}

func data(t *testing.T, e envelope) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(e.Data, &m); err != nil {
		t.Fatalf("bad data %s", e.Data)
	}
	return m
}

// ---------- register ----------

func TestRegisterOpenEventHappyPath(t *testing.T) {
	d, mock := deps(t, nil)
	got := capture(d)
	registerHead(mock, evRow("published"))
	expectMine(mock, playerID)
	expectInsert(mock, playerID, "registered", "open", nil, nil)
	expectBump(mock, 1)
	mock.ExpectCommit()
	code, env := post(t, d, map[string]any{"teamName": " Kings ", "teamMembers": []map[string]any{{"name": " Asha ", "role": "captain"}}})
	if code != http.StatusCreated {
		t.Fatalf("code %d %s", code, env.Data)
	}
	m := data(t, env)
	if m["status"] != "registered" || m["qualifiedVia"] != "open" || m["eligibilityFlag"] != "none" || m["teamName"] != "Kings" {
		t.Fatalf("bad body %v", m)
	}
	if m["qualifiedDistrict"] != nil || m["event"] != nil {
		t.Fatalf("district/event should be null: %v", m)
	}
	if len(*got) != 1 || (*got)[0].Type != events.RegistrationCreated {
		t.Fatalf("events %v", types(*got))
	}
	if p := payload(t, (*got)[0]); p.RegistrationID != regID || p.OrganiserID != orgID || p.Status != "registered" {
		t.Fatalf("payload %+v", p)
	}
}

func TestRegisterLocationScenarios(t *testing.T) {
	past := t0.Add(-time.Hour)
	future := t0.Add(90 * 24 * time.Hour)
	cases := []struct {
		name     string
		cfg      map[string]any
		ev       row
		ids      []ident
		loads    bool
		wantCode int
		via      string
		identity driver.Value
		district driver.Value
		reason   string
		action   string
	}{
		{name: "eligible native", ev: evRow("published").rule("native", "district", thanjavur, tamilNadu, true),
			ids: []ident{{id: 5, typ: "native", district: thanjavur, state: tamilNadu, status: "verified"}}, loads: true,
			wantCode: 201, via: "native", identity: int64(5), district: thanjavur},
		{name: "eligible residence", ev: evRow("published").rule("residence", "district", coimbatore, tamilNadu, true),
			ids: []ident{{id: 6, typ: "residence", district: coimbatore, state: tamilNadu, status: "verified", expires: &future}}, loads: true,
			wantCode: 201, via: "residence", identity: int64(6), district: coimbatore},
		{name: "ineligible wrong place", ev: evRow("published").rule("native", "district", thanjavur, tamilNadu, true),
			ids: []ident{{id: 5, typ: "native", district: ernakulam, state: 12, status: "verified"}}, loads: true,
			wantCode: 403, reason: "wrong_location", action: "verify_native"},
		{name: "ineligible missing", ev: evRow("published").rule("residence", "district", coimbatore, tamilNadu, true),
			loads: true, wantCode: 403, reason: "identity_missing", action: "verify_residence"},
		{name: "expired proof by date", ev: evRow("published").rule("residence", "district", coimbatore, tamilNadu, true),
			ids: []ident{{id: 6, typ: "residence", district: coimbatore, state: tamilNadu, status: "verified", expires: &past}}, loads: true,
			wantCode: 403, reason: "identity_expired", action: "renew_residence"},
		{name: "pending under require_verified true", ev: evRow("published").rule("native", "district", thanjavur, tamilNadu, true),
			ids: []ident{{id: 5, typ: "native", district: thanjavur, state: tamilNadu, status: "pending"}}, loads: true,
			wantCode: 403, reason: "identity_pending", action: "wait_for_review"},
		{name: "pending under require_verified false", ev: evRow("published").rule("native", "district", thanjavur, tamilNadu, false),
			ids: []ident{{id: 5, typ: "native", district: thanjavur, state: tamilNadu, status: "pending"}}, loads: true,
			wantCode: 201, via: "native", identity: int64(5), district: thanjavur},
		{name: "either via residence", ev: evRow("published").rule("either", "district", coimbatore, tamilNadu, true),
			ids: []ident{{id: 6, typ: "residence", district: coimbatore, state: tamilNadu, status: "verified", expires: &future}}, loads: true,
			wantCode: 201, via: "residence", identity: int64(6), district: coimbatore},
		{name: "state scope", ev: evRow("published").rule("native", "state", 0, tamilNadu, true),
			ids: []ident{{id: 5, typ: "native", district: coimbatore, state: tamilNadu, status: "verified"}}, loads: true,
			wantCode: 201, via: "native", identity: int64(5), district: coimbatore},
		{name: "switch off", cfg: map[string]any{"location.enabled": false},
			ev:       evRow("published").rule("native", "district", thanjavur, tamilNadu, true),
			wantCode: 201, via: "open"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, mock := deps(t, tc.cfg)
			got := capture(d)
			registerHead(mock, tc.ev)
			if tc.loads {
				expectIdentities(mock, playerID, tc.ids...)
			}
			if tc.wantCode == 201 {
				expectMine(mock, playerID)
				expectInsert(mock, playerID, "registered", tc.via, tc.identity, tc.district)
				expectBump(mock, 1)
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			code, env := post(t, d, map[string]any{})
			if code != tc.wantCode {
				t.Fatalf("code %d %s %+v", code, env.Data, env.Error)
			}
			if tc.wantCode == 403 {
				if env.Error.Code != "NOT_ELIGIBLE" || env.Error.Fields["reason"] != tc.reason || env.Error.Fields["action"] != tc.action {
					t.Fatalf("error %+v", env.Error)
				}
				if len(*got) != 0 {
					t.Fatalf("no events expected, got %v", types(*got))
				}
				return
			}
			m := data(t, env)
			if m["qualifiedVia"] != tc.via {
				t.Fatalf("qualifiedVia %v", m["qualifiedVia"])
			}
			if tc.district != nil {
				dist, _ := m["qualifiedDistrict"].(map[string]any)
				if dist == nil || int64(dist["id"].(float64)) != tc.district.(int64) || dist["name"] == "" {
					t.Fatalf("qualifiedDistrict %v", m["qualifiedDistrict"])
				}
			}
		})
	}
}

func TestRegisterRules(t *testing.T) {
	t.Run("own event 403", func(t *testing.T) {
		d, mock := deps(t, nil)
		mock.ExpectBegin()
		expectEvent(mock, evRow("published"), true)
		mock.ExpectRollback()
		w := testutil.Request(engine(d, orgID), http.MethodPost, "/v1/events/9/registrations", map[string]any{}, nil)
		if w.Code != http.StatusForbidden {
			t.Fatalf("code %d", w.Code)
		}
	})
	t.Run("missing event 404", func(t *testing.T) {
		d, mock := deps(t, nil)
		mock.ExpectBegin()
		mock.ExpectQuery(`FROM events e WHERE e.id = \? FOR UPDATE`).WithArgs(evID).WillReturnRows(sqlmock.NewRows(evColNames))
		mock.ExpectRollback()
		if code, _ := post(t, d, map[string]any{}); code != http.StatusNotFound {
			t.Fatalf("code %d", code)
		}
	})
	t.Run("draft event 404", func(t *testing.T) {
		d, mock := deps(t, nil)
		mock.ExpectBegin()
		expectEvent(mock, evRow("draft"), true)
		mock.ExpectRollback()
		if code, _ := post(t, d, map[string]any{}); code != http.StatusNotFound {
			t.Fatalf("code %d", code)
		}
	})
	t.Run("cancelled event 409", func(t *testing.T) {
		d, mock := deps(t, nil)
		mock.ExpectBegin()
		expectEvent(mock, evRow("cancelled"), true)
		mock.ExpectRollback()
		if code, env := post(t, d, map[string]any{}); code != http.StatusConflict || env.Error.Code != "INVALID_STATE" {
			t.Fatalf("code %d %+v", code, env.Error)
		}
	})
	t.Run("blocked 404", func(t *testing.T) {
		d, mock := deps(t, nil)
		mock.ExpectBegin()
		expectEvent(mock, evRow("published"), true)
		expectBlocked(mock, playerID, orgID, 1)
		mock.ExpectRollback()
		if code, _ := post(t, d, map[string]any{}); code != http.StatusNotFound {
			t.Fatalf("code %d", code)
		}
	})
	t.Run("deadline passed 409", func(t *testing.T) {
		d, mock := deps(t, nil)
		ev := evRow("published")
		ev["registration_deadline"] = t0.Add(-time.Minute)
		mock.ExpectBegin()
		expectEvent(mock, ev, true)
		expectBlocked(mock, playerID, orgID, 0)
		mock.ExpectRollback()
		if code, env := post(t, d, map[string]any{}); code != http.StatusConflict || env.Error.Code != "INVALID_STATE" {
			t.Fatalf("code %d %+v", code, env.Error)
		}
	})
	t.Run("age above max 403", func(t *testing.T) {
		d, mock := deps(t, nil)
		ev := evRow("published")
		ev["age_max"] = int64(16)
		mock.ExpectBegin()
		expectEvent(mock, ev, true)
		expectBlocked(mock, playerID, orgID, 0)
		expectFacts(mock, playerID, int64(2000), nil)
		mock.ExpectRollback()
		if code, env := post(t, d, map[string]any{}); code != http.StatusForbidden || env.Error.Code != "NOT_ELIGIBLE" ||
			env.Error.Fields["reason"] != "age_group" || env.Error.Fields["action"] != "none" {
			t.Fatalf("code %d %+v", code, env.Error)
		}
	})
	t.Run("unknown age and gender allowed", func(t *testing.T) {
		d, mock := deps(t, nil)
		ev := evRow("published")
		ev["age_max"], ev["gender_category"] = int64(16), "female"
		mock.ExpectBegin()
		expectEvent(mock, ev, true)
		expectBlocked(mock, playerID, orgID, 0)
		expectFacts(mock, playerID, nil, nil)
		expectMine(mock, playerID)
		expectInsert(mock, playerID, "registered", "open", nil, nil)
		expectBump(mock, 1)
		mock.ExpectCommit()
		if code, _ := post(t, d, map[string]any{}); code != http.StatusCreated {
			t.Fatalf("code %d", code)
		}
	})
	t.Run("gender mismatch 403", func(t *testing.T) {
		d, mock := deps(t, nil)
		ev := evRow("published")
		ev["gender_category"] = "female"
		mock.ExpectBegin()
		expectEvent(mock, ev, true)
		expectBlocked(mock, playerID, orgID, 0)
		expectFacts(mock, playerID, int64(2010), "M")
		mock.ExpectRollback()
		if code, env := post(t, d, map[string]any{}); code != http.StatusForbidden || env.Error.Code != "NOT_ELIGIBLE" ||
			env.Error.Fields["reason"] != "gender_category" || env.Error.Fields["action"] != "none" || env.Error.Message != "This event is for female players only." {
			t.Fatalf("code %d %+v", code, env.Error)
		}
	})
	t.Run("second registration 409 CONFLICT", func(t *testing.T) {
		d, mock := deps(t, nil)
		registerHead(mock, evRow("published"))
		expectMine(mock, playerID, regRow(regID, playerID, "waitlisted"))
		mock.ExpectRollback()
		if code, env := post(t, d, map[string]any{}); code != http.StatusConflict || env.Error.Code != "CONFLICT" {
			t.Fatalf("code %d %+v", code, env.Error)
		}
	})
	t.Run("withdrawn row is reactivated", func(t *testing.T) {
		d, mock := deps(t, nil)
		registerHead(mock, evRow("published"))
		expectMine(mock, playerID, regRow(regID, playerID, "withdrawn"))
		mock.ExpectExec(`UPDATE event_registrations SET team_name = \?, team_members = \?, status = \?`).
			WithArgs(nil, "[]", "registered", "open", nil, nil, regID).WillReturnResult(sqlmock.NewResult(0, 1))
		expectBump(mock, 1)
		mock.ExpectCommit()
		code, env := post(t, d, map[string]any{})
		if code != http.StatusCreated || int64(data(t, env)["id"].(float64)) != regID {
			t.Fatalf("code %d %s", code, env.Data)
		}
	})
	t.Run("full goes to waitlist", func(t *testing.T) {
		d, mock := deps(t, nil)
		ev := evRow("published")
		ev["registrations_count"] = int64(16)
		registerHead(mock, ev)
		expectMine(mock, playerID)
		expectInsert(mock, playerID, "waitlisted", "open", nil, nil)
		mock.ExpectCommit() // no registrations_count change for the waitlist
		code, env := post(t, d, map[string]any{})
		if code != http.StatusCreated || data(t, env)["status"] != "waitlisted" {
			t.Fatalf("code %d %s", code, env.Data)
		}
	})
	t.Run("full without waitlist 409 LIMIT_REACHED", func(t *testing.T) {
		d, mock := deps(t, map[string]any{"registration.waitlist_enabled": false})
		ev := evRow("published")
		ev["registrations_count"] = int64(16)
		registerHead(mock, ev)
		expectMine(mock, playerID)
		mock.ExpectRollback()
		if code, env := post(t, d, map[string]any{}); code != http.StatusConflict || env.Error.Code != "LIMIT_REACHED" {
			t.Fatalf("code %d %+v", code, env.Error)
		}
	})
	t.Run("duplicate insert race 409", func(t *testing.T) {
		d, mock := deps(t, nil)
		registerHead(mock, evRow("published"))
		expectMine(mock, playerID)
		mock.ExpectExec(`INSERT INTO event_registrations`).WillReturnError(errors.New("Error 1062: Duplicate entry '9-1'"))
		mock.ExpectRollback()
		if code, env := post(t, d, map[string]any{}); code != http.StatusConflict || env.Error.Code != "CONFLICT" {
			t.Fatalf("code %d %+v", code, env.Error)
		}
	})
}

func TestRegisterValidation(t *testing.T) {
	d, _ := deps(t, nil)
	members := make([]map[string]any, 31)
	for i := range members {
		members[i] = map[string]any{"name": "P"}
	}
	members[0] = map[string]any{"name": " "}
	code, env := post(t, d, map[string]any{"teamName": strings.Repeat("x", 81), "teamMembers": members})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code %d", code)
	}
	for _, f := range []string{"teamName", "teamMembers", "teamMembers[0].name"} {
		if env.Error.Fields[f] == "" {
			t.Fatalf("missing field %s in %v", f, env.Error.Fields)
		}
	}
}

// ---------- withdraw + promotion ----------

func TestWithdrawPromotesFirstEligibleAndFlagsSkipped(t *testing.T) {
	d, mock := deps(t, nil)
	got := capture(d)
	ev := evRow("published").rule("native", "district", thanjavur, tamilNadu, true)
	ev["registrations_count"] = int64(16)
	mock.ExpectBegin()
	expectEvent(mock, ev, true)
	expectMine(mock, playerID, regRow(regID, playerID, "registered").via("native", 5, thanjavur))
	mock.ExpectExec(`UPDATE event_registrations SET status = \? WHERE id = \? AND status = \?`).
		WithArgs("withdrawn", regID, "registered").WillReturnResult(sqlmock.NewResult(0, 1))
	expectBump(mock, -1)
	mock.ExpectQuery(`WHERE r.event_id = \? AND r.status = 'waitlisted'\s+ORDER BY r.id ASC LIMIT \? FOR UPDATE`).
		WithArgs(evID, promoteScan).
		WillReturnRows(regRows(regRow(40, otherID, "waitlisted").via("native", 7, thanjavur), regRow(41, 4, "waitlisted").via("native", 8, thanjavur)))
	// waitlisted #40: identity 7 was revoked → skipped and flagged identity_revoked
	expectIdentities(mock, otherID)
	mock.ExpectQuery(`SELECT id, status FROM user_location_identities WHERE id IN \(\?\)`).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(int64(7), "revoked"))
	mock.ExpectExec(`UPDATE event_registrations SET eligibility_flag = \?, eligibility_flagged_at = \?\s+WHERE id = \? AND eligibility_flag <> \?`).
		WithArgs("identity_revoked", t0, int64(40), "identity_revoked").WillReturnResult(sqlmock.NewResult(0, 1))
	// waitlisted #41: still eligible → promoted
	expectIdentities(mock, int64(4), ident{id: 8, typ: "native", district: thanjavur, state: tamilNadu, status: "verified"})
	mock.ExpectExec(`UPDATE event_registrations SET status = \?, qualified_via = \?, qualified_identity_id = \?`).
		WithArgs("registered", "native", int64(8), thanjavur, int64(41)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectBump(mock, 1)
	mock.ExpectCommit()

	w := testutil.Request(engine(d, playerID), http.MethodDelete, "/v1/events/9/registrations/me", nil, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("code %d %s", w.Code, w.Body.String())
	}
	want := []string{events.RegistrationWithdrawn, events.RegistrationPromoted, events.RegistrationEligibilityFlagged}
	if strings.Join(types(*got), ",") != strings.Join(want, ",") {
		t.Fatalf("events %v", types(*got))
	}
	if p := payload(t, (*got)[1]); p.RegistrationID != 41 || p.Status != "registered" {
		t.Fatalf("promoted %+v", p)
	}
	if p := payload(t, (*got)[2]); p.RegistrationID != 40 || p.EligibilityFlag != "identity_revoked" {
		t.Fatalf("flagged %+v", p)
	}
}

func TestWithdrawWaitlistedDoesNotPromote(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectBegin()
	expectEvent(mock, evRow("published"), true)
	expectMine(mock, playerID, regRow(regID, playerID, "waitlisted"))
	mock.ExpectExec(`UPDATE event_registrations SET status = \?`).WithArgs("withdrawn", regID, "waitlisted").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	w := testutil.Request(engine(d, playerID), http.MethodDelete, "/v1/events/9/registrations/me", nil, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("code %d", w.Code)
	}
}

func TestWithdrawWindowAndMissing(t *testing.T) {
	t.Run("inside withdraw window 409", func(t *testing.T) {
		d, mock := deps(t, map[string]any{"registration.withdraw_until_hours": 48})
		ev := evRow("published")
		ev["starts_at"] = t0.Add(47 * time.Hour)
		mock.ExpectBegin()
		expectEvent(mock, ev, true)
		expectMine(mock, playerID, regRow(regID, playerID, "registered"))
		mock.ExpectRollback()
		w := testutil.Request(engine(d, playerID), http.MethodDelete, "/v1/events/9/registrations/me", nil, nil)
		if env := decode(t, w.Body.Bytes()); w.Code != http.StatusConflict || env.Error.Code != "INVALID_STATE" {
			t.Fatalf("code %d %+v", w.Code, env.Error)
		}
	})
	t.Run("default 24h allows 47h before", func(t *testing.T) {
		d, mock := deps(t, nil)
		ev := evRow("published")
		ev["starts_at"] = t0.Add(47 * time.Hour)
		mock.ExpectBegin()
		expectEvent(mock, ev, true)
		expectMine(mock, playerID, regRow(regID, playerID, "waitlisted"))
		mock.ExpectExec(`UPDATE event_registrations SET status = \?`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		w := testutil.Request(engine(d, playerID), http.MethodDelete, "/v1/events/9/registrations/me", nil, nil)
		if w.Code != http.StatusNoContent {
			t.Fatalf("code %d", w.Code)
		}
	})
	t.Run("no registration 404", func(t *testing.T) {
		d, mock := deps(t, nil)
		mock.ExpectBegin()
		expectEvent(mock, evRow("published"), true)
		expectMine(mock, playerID)
		mock.ExpectRollback()
		w := testutil.Request(engine(d, playerID), http.MethodDelete, "/v1/events/9/registrations/me", nil, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("code %d", w.Code)
		}
	})
}

// ---------- organiser ----------

func TestOrganiserListOwnershipAndLabels(t *testing.T) {
	t.Run("not the owner 404", func(t *testing.T) {
		d, mock := deps(t, nil)
		expectEvent(mock, evRow("published"), false)
		w := testutil.Request(engine(d, otherID), http.MethodGet, "/v1/events/9/registrations", nil, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("code %d", w.Code)
		}
	})
	t.Run("bad status 422", func(t *testing.T) {
		d, _ := deps(t, nil)
		w := testutil.Request(engine(d, orgID), http.MethodGet, "/v1/events/9/registrations?status=nope", nil, nil)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("code %d", w.Code)
		}
	})
	t.Run("flagged filter and labels", func(t *testing.T) {
		d, mock := deps(t, nil)
		expectEvent(mock, evRow("published"), false)
		flaggedRow := regRow(40, otherID, "registered").via("residence", 8, coimbatore)
		flaggedRow["eligibility_flag"], flaggedRow["eligibility_flagged_at"] = "identity_expired", t0
		mock.ExpectQuery(`WHERE r.event_id = \? AND r.eligibility_flag <> 'none' ORDER BY r.id ASC LIMIT \?`).
			WithArgs(evID, 21).WillReturnRows(regRows(regRow(31, playerID, "registered").via("native", 5, thanjavur), flaggedRow, regRow(42, 4, "registered")))
		expectSummaries(mock, playerID, otherID, 4)
		mock.ExpectQuery(`SELECT id, status FROM user_location_identities WHERE id IN \(\?,\?\)`).WithArgs(int64(5), int64(8)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(int64(5), "verified").AddRow(int64(8), "pending"))
		w := testutil.Request(engine(d, orgID), http.MethodGet, "/v1/events/9/registrations?flagged=true", nil, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("code %d %s", w.Code, w.Body.String())
		}
		var items []map[string]any
		_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &items)
		want := []string{"Native: Thanjavur (verified)", "Residence: Coimbatore (in review)", "Open"}
		for i, l := range want {
			if items[i]["qualifiedLabel"] != l {
				t.Fatalf("label %d = %v", i, items[i]["qualifiedLabel"])
			}
			if _, has := items[i]["event"]; has {
				t.Fatalf("organiser view must not carry event")
			}
			if items[i]["user"] == nil {
				t.Fatalf("missing user")
			}
		}
		if items[1]["eligibilityFlag"] != "identity_expired" || items[1]["eligibilityFlaggedAt"] == nil {
			t.Fatalf("flag fields %v", items[1])
		}
	})
}

func TestLabel(t *testing.T) {
	r := rec{QualifiedVia: "native"}
	if got := label(r, "Thanjavur", "revoked"); got != "Native: Thanjavur (no longer valid)" {
		t.Fatal(got)
	}
	if got := label(rec{QualifiedVia: "residence"}, "Coimbatore", "expired"); got != "Residence: Coimbatore (expired)" {
		t.Fatal(got)
	}
}

func TestPatch(t *testing.T) {
	t.Run("validation 422", func(t *testing.T) {
		d, _ := deps(t, nil)
		w := testutil.Request(engine(d, orgID), http.MethodPatch, "/v1/events/9/registrations/31",
			map[string]any{"paymentStatus": "cash", "paymentNote": strings.Repeat("n", 201), "status": "registered"}, nil)
		env := decode(t, w.Body.Bytes())
		if w.Code != http.StatusUnprocessableEntity || len(env.Error.Fields) != 3 {
			t.Fatalf("code %d %v", w.Code, env.Error.Fields)
		}
	})
	t.Run("empty body 422", func(t *testing.T) {
		d, _ := deps(t, nil)
		w := testutil.Request(engine(d, orgID), http.MethodPatch, "/v1/events/9/registrations/31", map[string]any{}, nil)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("code %d", w.Code)
		}
	})
	t.Run("not the owner 404", func(t *testing.T) {
		d, mock := deps(t, nil)
		mock.ExpectBegin()
		expectEvent(mock, evRow("published"), true)
		mock.ExpectRollback()
		w := testutil.Request(engine(d, otherID), http.MethodPatch, "/v1/events/9/registrations/31", map[string]any{"paymentStatus": "waived"}, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("code %d", w.Code)
		}
	})
	t.Run("registration on another event 404", func(t *testing.T) {
		d, mock := deps(t, nil)
		mock.ExpectBegin()
		expectEvent(mock, evRow("published"), true)
		mock.ExpectQuery(`FROM event_registrations r WHERE r.id = \? AND r.event_id = \? FOR UPDATE`).WithArgs(regID, evID).
			WillReturnRows(regRows())
		mock.ExpectRollback()
		w := testutil.Request(engine(d, orgID), http.MethodPatch, "/v1/events/9/registrations/31", map[string]any{"paymentStatus": "waived"}, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("code %d", w.Code)
		}
	})
	t.Run("mark payment emits payment_marked", func(t *testing.T) {
		d, mock := deps(t, nil)
		got := capture(d)
		mock.ExpectBegin()
		expectEvent(mock, evRow("published"), true)
		mock.ExpectQuery(`WHERE r.id = \? AND r.event_id = \? FOR UPDATE`).WithArgs(regID, evID).
			WillReturnRows(regRows(regRow(regID, playerID, "registered")))
		mock.ExpectExec(`UPDATE event_registrations SET payment_status = \?, payment_note = \? WHERE id = \?`).
			WithArgs("paid_offline", "Cash at desk", regID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		expectSummaries(mock, playerID)
		w := testutil.Request(engine(d, orgID), http.MethodPatch, "/v1/events/9/registrations/31",
			map[string]any{"paymentStatus": "paid_offline", "paymentNote": " Cash at desk "}, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("code %d %s", w.Code, w.Body.String())
		}
		m := data(t, decode(t, w.Body.Bytes()))
		if m["paymentStatus"] != "paid_offline" || m["paymentNote"] != "Cash at desk" || m["qualifiedLabel"] != "Open" {
			t.Fatalf("body %v", m)
		}
		if strings.Join(types(*got), ",") != events.RegistrationPaymentMark {
			t.Fatalf("events %v", types(*got))
		}
	})
	t.Run("reject frees the slot and emits withdrawn with status rejected", func(t *testing.T) {
		d, mock := deps(t, nil)
		got := capture(d)
		mock.ExpectBegin()
		expectEvent(mock, evRow("published"), true)
		mock.ExpectQuery(`WHERE r.id = \? AND r.event_id = \? FOR UPDATE`).WithArgs(regID, evID).
			WillReturnRows(regRows(regRow(regID, playerID, "registered")))
		mock.ExpectExec(`UPDATE event_registrations SET status = \? WHERE id = \? AND status = \?`).
			WithArgs("rejected", regID, "registered").WillReturnResult(sqlmock.NewResult(0, 1))
		expectBump(mock, -1)
		mock.ExpectQuery(`r.status = 'waitlisted'`).WithArgs(evID, promoteScan).WillReturnRows(regRows())
		mock.ExpectCommit()
		expectSummaries(mock, playerID)
		w := testutil.Request(engine(d, orgID), http.MethodPatch, "/v1/events/9/registrations/31", map[string]any{"status": "rejected"}, nil)
		if w.Code != http.StatusOK || data(t, decode(t, w.Body.Bytes()))["status"] != "rejected" {
			t.Fatalf("code %d %s", w.Code, w.Body.String())
		}
		if len(*got) != 1 || (*got)[0].Type != events.RegistrationWithdrawn || payload(t, (*got)[0]).Status != "rejected" {
			t.Fatalf("events %v", types(*got))
		}
	})
	t.Run("reject withdrawn 409", func(t *testing.T) {
		d, mock := deps(t, nil)
		mock.ExpectBegin()
		expectEvent(mock, evRow("published"), true)
		mock.ExpectQuery(`WHERE r.id = \? AND r.event_id = \? FOR UPDATE`).WithArgs(regID, evID).
			WillReturnRows(regRows(regRow(regID, playerID, "withdrawn")))
		mock.ExpectRollback()
		w := testutil.Request(engine(d, orgID), http.MethodPatch, "/v1/events/9/registrations/31", map[string]any{"status": "rejected"}, nil)
		if w.Code != http.StatusConflict {
			t.Fatalf("code %d", w.Code)
		}
	})
}

// ---------- /me/registrations ----------

var eventCols = strings.Split("id,organiser_id,sport_id,s_code,s_name,title,description,event_type,participant_type,"+
	"team_size_min,team_size_max,competition_level,category,eligibility_type,eligibility_scope,"+
	"eligibility_district_id,eligibility_state_id,require_verified,gender_category,age_min,age_max,venue_name,"+
	"address_line,city,district,state,pincode,latitude,longitude,starts_at,ends_at,registration_deadline,"+
	"entry_fee_paise,payment_modes,payment_instructions,max_slots,rules,prize_info,contact_name,"+
	"contact_phone_last4,banner_upload_id,status,rejection_reason,cancel_reason,registrations_count,"+
	"featured_at,published_at,created_at", ",")

func TestMineIncludesEventSummary(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectQuery(`FROM event_registrations r WHERE r.user_id = \? AND r.status = \? ORDER BY r.id DESC LIMIT \?`).
		WithArgs(playerID, "registered", 2).
		WillReturnRows(regRows(regRow(32, playerID, "registered"), regRow(31, playerID, "registered")))
	ev := row{"id": evID, "organiser_id": orgID, "sport_id": int64(1), "s_code": "cricket", "s_name": "Cricket",
		"title": "Pune Cup", "event_type": "tournament", "participant_type": "individual", "competition_level": "open",
		"category": "private", "eligibility_type": "open", "require_verified": true, "gender_category": "open",
		"venue_name": "Ground", "city": "Pune", "state": "Maharashtra", "starts_at": starts,
		"ends_at": starts.Add(8 * time.Hour), "registration_deadline": starts.Add(-24 * time.Hour),
		"entry_fee_paise": int64(50000), "payment_modes": []byte(`["upi"]`), "max_slots": int64(16),
		"contact_name": "Ravi", "contact_phone_last4": "3210", "status": "published", "registrations_count": int64(3), "created_at": t0}
	mock.ExpectQuery(`FROM events e JOIN sports s ON s.id = e.sport_id WHERE e.id IN \(\?\)`).WithArgs(evID).
		WillReturnRows(sqlmock.NewRows(eventCols).AddRow(vals(eventCols, ev)...))
	expectSummaries(mock, orgID)
	w := testutil.Request(engine(d, playerID), http.MethodGet, "/v1/me/registrations?status=registered&limit=1", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d %s", w.Code, w.Body.String())
	}
	env := decode(t, w.Body.Bytes())
	var items []map[string]any
	_ = json.Unmarshal(env.Data, &items)
	if len(items) != 1 || env.Meta["nextCursor"] == "" || env.Meta["nextCursor"] == nil {
		t.Fatalf("page %s %v", env.Data, env.Meta)
	}
	e, _ := items[0]["event"].(map[string]any)
	if e == nil || e["title"] != "Pune Cup" || e["slotsLeft"].(float64) != 13 || e["myEligibility"] == nil {
		t.Fatalf("event %v", items[0]["event"])
	}
}

func TestMineBadStatus(t *testing.T) {
	d, _ := deps(t, nil)
	w := testutil.Request(engine(d, playerID), http.MethodGet, "/v1/me/registrations?status=x", nil, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code %d", w.Code)
	}
}

// ---------- location_identity.changed ----------

func identityEvent(status string) events.Event {
	return events.New(events.LocationIdentityChanged, 0, events.LocationIdentityPayload{IdentityID: 5, UserID: playerID,
		IdentityType: "native", DistrictID: thanjavur, Status: status, PrevStatus: "verified"})
}

var joinCols = append(append([]string{}, regColNames...), evColNames...)

func joinRows(pairs ...[2]row) *sqlmock.Rows {
	out := sqlmock.NewRows(joinCols)
	for _, p := range pairs {
		out.AddRow(append(vals(regColNames, p[0]), vals(evColNames, p[1])...)...)
	}
	return out
}

func expectByIdentity(mock sqlmock.Sqlmock, after int64, rows *sqlmock.Rows) {
	mock.ExpectQuery(`JOIN events e ON e.id = r.event_id\s+WHERE r.user_id = \? AND r.qualified_identity_id = \? AND r.status IN \('registered','waitlisted'\)\s+AND e.status = 'published' AND e.starts_at > \? AND r.id > \?\s+ORDER BY r.id ASC LIMIT \? FOR UPDATE`).
		WithArgs(playerID, int64(5), t0, after, batchSize).WillReturnRows(rows)
}

func TestIdentityChangedRequalifiesOrFlagsAndNeverCancels(t *testing.T) {
	d, mock := deps(t, nil)
	got := capture(d)
	h := New(d).EventHandlers()[events.LocationIdentityChanged]
	stateRule := evRow("published").rule("either", "state", 0, tamilNadu, true)
	districtRule := evRow("published").rule("native", "district", thanjavur, tamilNadu, true)
	districtRule["id"], districtRule["organiser_id"] = int64(10), int64(6)

	mock.ExpectBegin()
	expectByIdentity(mock, 0, joinRows(
		[2]row{regRow(31, playerID, "registered").via("native", 5, thanjavur), stateRule},
		[2]row{regRow(33, playerID, "waitlisted").via("native", 5, thanjavur), districtRule},
	))
	// identity 5 is now revoked (not loaded); a verified residence in Coimbatore remains.
	future := t0.Add(30 * 24 * time.Hour)
	expectIdentities(mock, playerID, ident{id: 8, typ: "residence", district: coimbatore, state: tamilNadu, status: "verified", expires: &future})
	mock.ExpectExec(`UPDATE event_registrations SET status = \?, qualified_via = \?, qualified_identity_id = \?`).
		WithArgs("registered", "residence", int64(8), coimbatore, int64(31)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE event_registrations SET eligibility_flag = \?, eligibility_flagged_at = \?`).
		WithArgs("identity_revoked", t0, int64(33), "identity_revoked").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := h(context.Background(), identityEvent("revoked")); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || (*got)[0].Type != events.RegistrationEligibilityFlagged {
		t.Fatalf("events %v", types(*got))
	}
	p := payload(t, (*got)[0])
	if p.RegistrationID != 33 || p.EligibilityFlag != "identity_revoked" || p.OrganiserID != 6 || p.Status != "waitlisted" {
		t.Fatalf("payload %+v", p)
	}

	// Re-delivery: the requalified row no longer matches; the flagged one is already flagged.
	*got = nil
	mock.ExpectBegin()
	expectByIdentity(mock, 0, joinRows([2]row{regRow(33, playerID, "waitlisted").via("native", 5, thanjavur), districtRule}))
	expectIdentities(mock, playerID, ident{id: 8, typ: "residence", district: coimbatore, state: tamilNadu, status: "verified", expires: &future})
	mock.ExpectExec(`UPDATE event_registrations SET eligibility_flag = \?`).
		WithArgs("identity_revoked", t0, int64(33), "identity_revoked").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err := h(context.Background(), identityEvent("revoked")); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 0 {
		t.Fatalf("re-delivery must not publish, got %v", types(*got))
	}
}

func TestIdentityChangedFlagMapping(t *testing.T) {
	for status, flag := range map[string]string{"expired": "identity_expired", "revoked": "identity_revoked",
		"withdrawn": "identity_changed", "superseded": "identity_changed", "rejected": "identity_rejected"} {
		t.Run(status, func(t *testing.T) {
			d, mock := deps(t, nil)
			got := capture(d)
			mock.ExpectBegin()
			expectByIdentity(mock, 0, joinRows([2]row{regRow(31, playerID, "registered").via("native", 5, thanjavur),
				evRow("published").rule("native", "district", thanjavur, tamilNadu, false)}))
			expectIdentities(mock, playerID)
			mock.ExpectExec(`SET eligibility_flag = \?`).WithArgs(flag, t0, int64(31), flag).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			if err := New(d).svc.OnIdentityChanged(context.Background(), identityEvent(status)); err != nil {
				t.Fatal(err)
			}
			if len(*got) != 1 || payload(t, (*got)[0]).EligibilityFlag != flag {
				t.Fatalf("events %v", types(*got))
			}
		})
	}
}

func TestIdentityChangedSkips(t *testing.T) {
	for name, tc := range map[string]struct {
		cfg    map[string]any
		status string
	}{
		"verified is not a loss": {status: "verified"},
		"pending is not a loss":  {status: "pending"},
		"setting off":            {cfg: map[string]any{"location.flag_registrations_on_change": false}, status: "expired"},
		"location off":           {cfg: map[string]any{"location.enabled": false}, status: "expired"},
	} {
		t.Run(name, func(t *testing.T) {
			d, _ := deps(t, tc.cfg) // no SQL expected at all
			if err := New(d).svc.OnIdentityChanged(context.Background(), identityEvent(tc.status)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestIdentityChangedBatches(t *testing.T) {
	d, mock := deps(t, nil)
	first := make([][2]row, batchSize)
	for i := range first {
		first[i] = [2]row{regRow(int64(i+1), playerID, "registered").via("native", 5, thanjavur), evRow("published").rule("native", "district", thanjavur, tamilNadu, true)}
	}
	mock.ExpectBegin()
	expectByIdentity(mock, 0, joinRows(first...))
	expectIdentities(mock, playerID)
	for i := 1; i <= batchSize; i++ {
		mock.ExpectExec(`SET eligibility_flag = \?`).WithArgs("identity_expired", t0, int64(i), "identity_expired").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectCommit()
	mock.ExpectBegin()
	expectByIdentity(mock, int64(batchSize), joinRows())
	mock.ExpectCommit()
	if err := New(d).svc.OnIdentityChanged(context.Background(), identityEvent("expired")); err != nil {
		t.Fatal(err)
	}
}

func TestEventHandlersRegistered(t *testing.T) {
	d, _ := deps(t, nil)
	if _, ok := New(d).EventHandlers()[events.LocationIdentityChanged]; !ok {
		t.Fatal("location_identity.changed handler missing")
	}
}

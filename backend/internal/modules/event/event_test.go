package event

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/requirements"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

const (
	playerID int64 = 1
	orgID    int64 = 2
	adminID  int64 = 7
	evID     int64 = 9

	thanjavur  int64 = 26
	coimbatore int64 = 4
	ernakulam  int64 = 9001
	tamilNadu  int64 = 23
	kerala     int64 = 12
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
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func decode(t *testing.T, b []byte) envelope {
	t.Helper()
	var e envelope
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

func organiser() userdir.Basic {
	u := testutil.VerifiedUser(orgID)
	u.PrimaryRole, u.Roles = "organiser", []string{"organiser"}
	return u
}

func userEngine(d *app.Deps, u userdir.Basic) http.Handler {
	r, routes := testutil.UserEngine(u)
	New(d).Routes(routes)
	return r
}

func adminEngine(d *app.Deps, perms ...string) http.Handler {
	r, routes := testutil.AdminEngine(adminID, perms)
	New(d).Routes(routes)
	return r
}

// capture records every events.* type published.
func capture(d *app.Deps) *[]events.Event {
	var got []events.Event
	for _, typ := range []string{events.EventUpdated, events.EventCancelled, events.EventPublished, events.EventRejected, events.EventCompleted} {
		d.Dispatcher.On(typ, func(_ context.Context, e events.Event) error { got = append(got, e); return nil })
	}
	return &got
}

var evCols = strings.Split("id,organiser_id,sport_id,s_code,s_name,title,description,event_type,participant_type,"+
	"team_size_min,team_size_max,competition_level,category,eligibility_type,eligibility_scope,"+
	"eligibility_district_id,eligibility_state_id,require_verified,gender_category,age_min,age_max,venue_name,"+
	"address_line,city,district,state,pincode,latitude,longitude,starts_at,ends_at,registration_deadline,"+
	"entry_fee_paise,payment_modes,payment_instructions,max_slots,rules,prize_info,contact_name,"+
	"contact_phone_last4,banner_upload_id,status,rejection_reason,cancel_reason,registrations_count,"+
	"featured_at,published_at,created_at", ",")

type row map[string]driver.Value

func eventRow(id, org int64, status string) row {
	return row{"id": id, "organiser_id": org, "sport_id": int64(1), "s_code": "cricket", "s_name": "Cricket",
		"title": "Pune Cup", "event_type": "tournament", "participant_type": "individual", "competition_level": "open",
		"category": "private", "eligibility_type": "open", "require_verified": true, "gender_category": "open",
		"venue_name": "Ground", "city": "Pune", "state": "Maharashtra", "starts_at": starts,
		"ends_at": starts.Add(8 * time.Hour), "registration_deadline": starts.Add(-24 * time.Hour),
		"entry_fee_paise": int64(50000), "payment_modes": []byte(`["upi"]`), "payment_instructions": "UPI to 98xxxx",
		"max_slots": int64(16), "contact_name": "Ravi", "contact_phone_last4": "3210", "status": status,
		"registrations_count": int64(3), "created_at": t0}
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

func evRows(withDist bool, rs ...row) *sqlmock.Rows {
	cols := evCols
	if withDist {
		cols = append(append([]string{}, evCols...), "dist")
	}
	out := sqlmock.NewRows(cols)
	for _, r := range rs {
		vals := make([]driver.Value, 0, len(cols))
		for _, c := range evCols {
			vals = append(vals, r[c])
		}
		if withDist {
			vals = append(vals, r["dist"])
		}
		out.AddRow(vals...)
	}
	return out
}

func expectGet(mock sqlmock.Sqlmock, r row, forUpdate bool) {
	re := `FROM events e JOIN sports s ON s.id = e.sport_id WHERE e.id = \?$`
	if forUpdate {
		re = `FROM events e JOIN sports s ON s.id = e.sport_id WHERE e.id = \? FOR UPDATE$`
	}
	mock.ExpectQuery(re).WithArgs(r["id"]).WillReturnRows(evRows(false, r))
}

func expectGetMissing(mock sqlmock.Sqlmock, id int64, forUpdate bool) {
	re := `WHERE e.id = \?$`
	if forUpdate {
		re = `WHERE e.id = \? FOR UPDATE$`
	}
	mock.ExpectQuery(re).WithArgs(id).WillReturnRows(sqlmock.NewRows(evCols))
}

func expectBlocked(mock sqlmock.Sqlmock, a, b int64, n int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blocks`).WithArgs(a, b, b, a).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func expectSummaries(mock sqlmock.Sqlmock, ids ...int64) {
	rows := sqlmock.NewRows([]string{"id", "dn", "pr", "vs", "photo", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"})
	args := []driver.Value{}
	for _, id := range ids {
		rows.AddRow(id, "Organiser", "organiser", "verified", nil, nil, nil, nil, nil, nil, nil, nil, nil)
		args = append(args, id)
	}
	mock.ExpectQuery(`FROM users u\s+LEFT JOIN profiles`).WithArgs(args...).WillReturnRows(rows)
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
	mock.ExpectQuery(`FROM user_location_identities WHERE user_id = \?`).WithArgs(uid).WillReturnRows(rows)
}

func expectMyRegistration(mock sqlmock.Sqlmock, eventID, uid int64, status string) {
	rows := sqlmock.NewRows([]string{"id", "eid", "st", "ps", "pn", "tn", "tm", "ca", "qv", "qd", "ef", "efa"})
	if status != "" {
		rows.AddRow(int64(40), eventID, status, "unpaid", nil, nil, nil, t0, "native", thanjavur, "none", nil)
	}
	mock.ExpectQuery(`FROM event_registrations WHERE event_id = \? AND user_id = \?`).WithArgs(eventID, uid).WillReturnRows(rows)
}

func expectGroupChat(mock sqlmock.Sqlmock, eventID int64, n int) {
	mock.ExpectQuery(`FROM conversations WHERE type = 'event'`).WithArgs(eventID).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func expectLatestListing(mock sqlmock.Sqlmock, eventID int64) {
	mock.ExpectQuery(`FROM listing_orders o\s+WHERE o.id IN`).WithArgs(eventID).
		WillReturnRows(sqlmock.NewRows([]string{"e", "id", "st", "t"}).AddRow(eventID, int64(12), "paid", int64(47200)))
}

// expectOwnerDetail is the tail of GET /events/:id for the owner (open event).
func expectOwnerDetail(mock sqlmock.Sqlmock, r row) {
	expectGet(mock, r, false)
	expectSummaries(mock, r["organiser_id"].(int64))
	expectGroupChat(mock, r["id"].(int64), 0)
	expectLatestListing(mock, r["id"].(int64))
}

// capt is an sqlmock argument that records the value.
type capt struct{ v *driver.Value }

func (c capt) Match(v driver.Value) bool { *c.v = v; return true }

func capArgs(n int) ([]driver.Value, []driver.Value) {
	vals := make([]driver.Value, n)
	args := make([]driver.Value, n)
	for i := range args {
		args[i] = capt{&vals[i]}
	}
	return vals, args
}

func validInput() map[string]any {
	return map[string]any{"sportId": 1, "title": "Pune U-16 Cricket Cup", "venueName": "Ground", "city": "Pune",
		"state": "Maharashtra", "pincode": "411001", "startsAt": starts, "endsAt": starts.Add(8 * time.Hour),
		"registrationDeadline": starts.Add(-24 * time.Hour), "entryFeePaise": 50000, "paymentModes": []string{"upi", "upi"},
		"paymentInstructions": "UPI", "maxSlots": 16, "contactName": "Ravi", "contactPhone": "+91 98765 43210"}
}

func expectApproved(mock sqlmock.Sqlmock, status string) {
	mock.ExpectQuery(`SELECT status FROM organiser_profiles WHERE user_id = \?`).WithArgs(orgID).
		WillReturnRows(sqlmock.NewRows([]string{"s"}).AddRow(status))
}

func expectSport(mock sqlmock.Sqlmock, n int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM sports WHERE id = \? AND active = 1`).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func expectCountActive(mock sqlmock.Sqlmock, n int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM events WHERE organiser_id = \?`).WithArgs(orgID).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

// ---------- create ----------

func TestCreateHappyPathEncryptsPhone(t *testing.T) {
	d, mock := deps(t, nil)
	expectApproved(mock, "approved")
	expectSport(mock, 1)
	mock.ExpectBegin()
	expectCountActive(mock, 2)
	vals, args := capArgs(40)
	mock.ExpectExec(`INSERT INTO events`).WithArgs(args...).WillReturnResult(sqlmock.NewResult(evID, 1))
	mock.ExpectCommit()
	expectOwnerDetail(mock, eventRow(evID, orgID, stDraft))

	w := testutil.Request(userEngine(d, organiser()), "POST", "/v1/events", validInput(), nil)
	if w.Code != 201 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	for i, v := range vals {
		if s, ok := v.(string); ok && strings.Contains(s, "9876543210") {
			t.Fatalf("plain phone stored at arg %d", i)
		}
	}
	if vals[9] != "private" || vals[10] != "open" || vals[14] != true {
		t.Fatalf("defaults: category=%v type=%v requireVerified=%v", vals[9], vals[10], vals[14])
	}
	if vals[30] != `["upi"]` {
		t.Fatalf("payment modes deduped: %v", vals[30])
	}
	enc, ok := vals[36].([]byte)
	if !ok || len(enc) == 0 {
		t.Fatalf("phone not encrypted")
	}
	if plain, err := d.Crypto.DecryptString(enc); err != nil || plain != "9876543210" {
		t.Fatalf("decrypt %q %v", plain, err)
	}
	if vals[37] != "3210" {
		t.Fatalf("last4 %v", vals[37])
	}
	var ev Event
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &ev)
	if ev.ContactPhoneMasked != "******3210" || !ev.IsOwner || ev.Listing == nil || ev.Eligibility.Summary == "" {
		t.Fatalf("event %+v", ev)
	}
	if ev.MyEligibility == nil || !ev.MyEligibility.Eligible || *ev.MyEligibility.Via != "open" {
		t.Fatalf("myEligibility %+v", ev.MyEligibility)
	}
}

func TestCreateNeedsOrganiserRole(t *testing.T) {
	d, _ := deps(t, nil)
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(playerID)), "POST", "/v1/events", validInput(), nil)
	if w.Code != 403 || decode(t, w.Body.Bytes()).Error.Code != httpx.CodeForbidden {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestCreateOrganiserNotApproved(t *testing.T) {
	d, mock := deps(t, nil)
	expectApproved(mock, "pending")
	w := testutil.Request(userEngine(d, organiser()), "POST", "/v1/events", validInput(), nil)
	if w.Code != 403 || decode(t, w.Body.Bytes()).Error.Code != httpx.CodeOrganiserPending {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestCreateApprovalNotRequired(t *testing.T) {
	d, mock := deps(t, nil)
	m := map[string]bool{}
	for k, v := range testutil.DefaultRequirements {
		m[k] = v
	}
	m["organiser/organiser_approval/create_event"] = false
	d.Requirements = requirements.NewStatic(m)
	expectSport(mock, 1)
	mock.ExpectBegin()
	expectCountActive(mock, 0)
	mock.ExpectExec(`INSERT INTO events`).WillReturnResult(sqlmock.NewResult(evID, 1))
	mock.ExpectCommit()
	expectOwnerDetail(mock, eventRow(evID, orgID, stDraft))
	if w := testutil.Request(userEngine(d, organiser()), "POST", "/v1/events", validInput(), nil); w.Code != 201 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestCreateLimitReached(t *testing.T) {
	d, mock := deps(t, map[string]any{"event.max_active_per_organiser": 3})
	expectApproved(mock, "approved")
	expectSport(mock, 1)
	mock.ExpectBegin()
	expectCountActive(mock, 3)
	mock.ExpectRollback()
	w := testutil.Request(userEngine(d, organiser()), "POST", "/v1/events", validInput(), nil)
	if w.Code != 409 || decode(t, w.Body.Bytes()).Error.Code != httpx.CodeLimitReached {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestCreateValidation(t *testing.T) {
	d, mock := deps(t, map[string]any{"event.allow_free_events": false, "event.max_slots_limit": 64})
	expectApproved(mock, "approved")
	expectSport(mock, 0)
	in := validInput()
	in["title"] = "ab"
	in["pincode"] = "011001"
	in["participantType"] = "team"
	in["teamSizeMin"], in["teamSizeMax"] = 5, 40
	in["entryFeePaise"] = 0
	in["maxSlots"] = 65
	in["startsAt"] = t0.Add(2 * time.Hour)
	in["contactPhone"] = "12345"
	in["latitude"] = 18.5
	in["category"] = "ngo"
	in["paymentModes"] = []string{"cheque"}
	in["eligibility"] = map[string]any{"type": "native", "scope": "district"}
	w := testutil.Request(userEngine(d, organiser()), "POST", "/v1/events", in, nil)
	if w.Code != 422 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	f := decode(t, w.Body.Bytes()).Error.Fields
	for _, k := range []string{"title", "pincode", "teamSizeMin", "entryFeePaise", "maxSlots", "startsAt",
		"contactPhone", "latitude", "category", "paymentModes", "eligibility.districtId", "sportId", "registrationDeadline"} {
		if f[k] == "" {
			t.Errorf("missing field error %q in %v", k, f)
		}
	}
}

func TestCreateEligibilityDistrictDerivesState(t *testing.T) {
	d, mock := deps(t, nil)
	expectApproved(mock, "approved")
	expectSport(mock, 1)
	mock.ExpectBegin()
	expectCountActive(mock, 0)
	vals, args := capArgs(40)
	mock.ExpectExec(`INSERT INTO events`).WithArgs(args...).WillReturnResult(sqlmock.NewResult(evID, 1))
	mock.ExpectCommit()
	r := eventRow(evID, orgID, stDraft).rule("native", "district", thanjavur, tamilNadu, true)
	r["category"] = "government"
	expectGet(mock, r, false)
	expectSummaries(mock, orgID)
	expectIdentities(mock, orgID) // the owner sees their own myEligibility too
	expectGroupChat(mock, evID, 0)
	expectLatestListing(mock, evID)

	in := validInput()
	in["category"] = "government"
	// stateId from the client (Kerala) is ignored; Thanjavur's state (Tamil Nadu) is stored.
	in["eligibility"] = map[string]any{"type": "native", "scope": "district", "districtId": thanjavur, "stateId": kerala}
	w := testutil.Request(userEngine(d, organiser()), "POST", "/v1/events", in, nil)
	if w.Code != 201 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	if vals[9] != "government" || vals[10] != "native" || vals[11] != "district" || vals[12] != thanjavur || vals[13] != tamilNadu {
		t.Fatalf("stored rule %v %v %v %v %v", vals[9], vals[10], vals[11], vals[12], vals[13])
	}
	var ev Event
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &ev)
	if ev.Eligibility.District == nil || ev.Eligibility.District.Name != "Thanjavur" || ev.Eligibility.State == nil ||
		ev.Eligibility.State.ID != tamilNadu || !strings.Contains(ev.Eligibility.Summary, "Thanjavur") {
		t.Fatalf("rule view %+v", ev.Eligibility)
	}
}

func TestCreateEligibilityRejectsBadDistrictAndOpenWithScope(t *testing.T) {
	for name, el := range map[string]map[string]any{
		"inactive district": {"type": "native", "scope": "district", "districtId": 999},
		"open with scope":   {"type": "open", "scope": "district", "districtId": thanjavur},
		"bad type":          {"type": "anywhere"},
		"state missing":     {"type": "residence", "scope": "state"},
	} {
		t.Run(name, func(t *testing.T) {
			d, mock := deps(t, nil)
			expectApproved(mock, "approved")
			expectSport(mock, 1)
			in := validInput()
			in["eligibility"] = el
			w := testutil.Request(userEngine(d, organiser()), "POST", "/v1/events", in, nil)
			if w.Code != 422 {
				t.Fatalf("code %d %s", w.Code, w.Body)
			}
			for k := range decode(t, w.Body.Bytes()).Error.Fields {
				if !strings.HasPrefix(k, "eligibility.") {
					t.Fatalf("unexpected field %s", k)
				}
			}
		})
	}
}

// ---------- update ----------

func TestUpdateOtherOrganiser404(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, 99, stDraft), true)
	mock.ExpectRollback()
	w := testutil.Request(userEngine(d, organiser()), "PUT", "/v1/events/9", map[string]any{"title": "New title"}, nil)
	if w.Code != 404 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestUpdatePendingReview409(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPendingReview), true)
	mock.ExpectRollback()
	w := testutil.Request(userEngine(d, organiser()), "PUT", "/v1/events/9", map[string]any{"title": "New title"}, nil)
	if w.Code != 409 || decode(t, w.Body.Bytes()).Error.Code != httpx.CodeInvalidState {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestUpdatePublishedLocksFields(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPublished), true)
	expectSport(mock, 1)
	mock.ExpectRollback()
	w := testutil.Request(userEngine(d, organiser()), "PUT", "/v1/events/9", map[string]any{
		"entryFeePaise": 1000, "category": "government", "eligibility": map[string]any{"type": "native", "scope": "district", "districtId": thanjavur},
		"description": "ok to change"}, nil)
	if w.Code != 422 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	f := decode(t, w.Body.Bytes()).Error.Fields
	for _, k := range []string{"entryFeePaise", "category", "eligibility"} {
		if !strings.Contains(f[k], "locked") {
			t.Errorf("%s not locked: %v", k, f)
		}
	}
	if _, ok := f["description"]; ok {
		t.Fatalf("description should be editable")
	}
}

func TestUpdatePublishedAllowedFieldsEmitsUpdated(t *testing.T) {
	d, mock := deps(t, nil)
	got := capture(d)
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPublished), true)
	expectSport(mock, 1)
	mock.ExpectExec(`UPDATE events SET sport_id = \?`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectOwnerDetail(mock, eventRow(evID, orgID, stPublished))
	w := testutil.Request(userEngine(d, organiser()), "PUT", "/v1/events/9", map[string]any{"description": "More details", "contactPhone": "9123456789"}, nil)
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	if len(*got) != 1 || (*got)[0].Type != events.EventUpdated {
		t.Fatalf("events %+v", *got)
	}
}

func TestUpdatePendingPaymentReturnsToDraft(t *testing.T) {
	d, mock := deps(t, nil)
	got := capture(d)
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPendingPayment), true)
	expectSport(mock, 1)
	vals, args := capArgs(42)
	mock.ExpectExec(`UPDATE events SET sport_id = \?`).WithArgs(args...).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectOwnerDetail(mock, eventRow(evID, orgID, stDraft))
	w := testutil.Request(userEngine(d, organiser()), "PUT", "/v1/events/9", map[string]any{"maxSlots": 32}, nil)
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	// ..., banner, to, id, from
	if vals[39] != stDraft || vals[41] != stPendingPayment || vals[35] != nil || vals[31] != int64(32) {
		t.Fatalf("args to=%v from=%v enc=%v slots=%v", vals[39], vals[41], vals[35], vals[31])
	}
	if len(*got) != 1 || (*got)[0].Type != events.EventUpdated {
		t.Fatalf("events %+v", *got)
	}
}

// ---------- get / eligibility ----------

func TestGetDraftHiddenFromOthers(t *testing.T) {
	d, mock := deps(t, nil)
	expectGet(mock, eventRow(evID, orgID, stDraft), false)
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/events/9", nil, nil)
	if w.Code != 404 {
		t.Fatalf("code %d", w.Code)
	}
}

func TestGetBlocked404(t *testing.T) {
	d, mock := deps(t, nil)
	expectGet(mock, eventRow(evID, orgID, stPublished), false)
	expectBlocked(mock, playerID, orgID, 1)
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/events/9", nil, nil)
	if w.Code != 404 {
		t.Fatalf("code %d", w.Code)
	}
}

func TestGetMissing404(t *testing.T) {
	d, mock := deps(t, nil)
	expectGetMissing(mock, evID, false)
	if w := testutil.Request(userEngine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/events/9", nil, nil); w.Code != 404 {
		t.Fatalf("code %d", w.Code)
	}
}

// getAsPlayer runs GET /events/9 for a non-owner on event r with the given identities.
func getAsPlayer(t *testing.T, d *app.Deps, mock sqlmock.Sqlmock, r row, loads bool, ids ...ident) Event {
	t.Helper()
	expectGet(mock, r, false)
	expectBlocked(mock, playerID, orgID, 0)
	expectSummaries(mock, orgID)
	if loads {
		expectIdentities(mock, playerID, ids...)
	}
	expectMyRegistration(mock, evID, playerID, "")
	expectGroupChat(mock, evID, 1)
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/events/9", nil, nil)
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	var ev Event
	if err := json.Unmarshal(decode(t, w.Body.Bytes()).Data, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.MyEligibility == nil {
		t.Fatalf("myEligibility missing")
	}
	return ev
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func TestEligibilityScenarios(t *testing.T) {
	past := t0.Add(-time.Hour)
	future := t0.Add(90 * 24 * time.Hour)
	cases := []struct {
		name     string
		rule     row
		ids      []ident
		eligible bool
		via      string
		pending  bool
		reason   string
		tag      string
	}{
		{"eligible native", eventRow(evID, orgID, stPublished).rule("native", "district", thanjavur, tamilNadu, true),
			[]ident{{1, "native", thanjavur, tamilNadu, "verified", nil}}, true, "native", false, "", "Eligible via Native: Thanjavur"},
		{"eligible residence", eventRow(evID, orgID, stPublished).rule("residence", "district", coimbatore, tamilNadu, true),
			[]ident{{2, "residence", coimbatore, tamilNadu, "verified", &future}}, true, "residence", false, "", "Coimbatore"},
		{"ineligible wrong place", eventRow(evID, orgID, stPublished).rule("native", "district", thanjavur, tamilNadu, true),
			[]ident{{1, "native", ernakulam, kerala, "verified", nil}}, false, "", false, "wrong_location", ""},
		{"ineligible no identity", eventRow(evID, orgID, stPublished).rule("native", "district", thanjavur, tamilNadu, true),
			nil, false, "", false, "identity_missing", ""},
		{"expired proof (status)", eventRow(evID, orgID, stPublished).rule("residence", "district", coimbatore, tamilNadu, true),
			[]ident{{2, "residence", coimbatore, tamilNadu, "expired", &past}}, false, "", false, "identity_expired", ""},
		{"expired proof (date passed)", eventRow(evID, orgID, stPublished).rule("residence", "district", coimbatore, tamilNadu, true),
			[]ident{{2, "residence", coimbatore, tamilNadu, "verified", &past}}, false, "", false, "identity_expired", ""},
		{"pending, require_verified true", eventRow(evID, orgID, stPublished).rule("native", "district", thanjavur, tamilNadu, true),
			[]ident{{1, "native", thanjavur, tamilNadu, "pending", nil}}, false, "", false, "identity_pending", ""},
		{"pending, require_verified false", eventRow(evID, orgID, stPublished).rule("native", "district", thanjavur, tamilNadu, false),
			[]ident{{1, "native", thanjavur, tamilNadu, "pending", nil}}, true, "native", true, "", "(in review)"},
		{"either via residence", eventRow(evID, orgID, stPublished).rule("either", "district", coimbatore, tamilNadu, true),
			[]ident{{2, "residence", coimbatore, tamilNadu, "verified", &future}}, true, "residence", false, "", "Coimbatore"},
		{"state scope", eventRow(evID, orgID, stPublished).rule("native", "state", 0, tamilNadu, true),
			[]ident{{1, "native", coimbatore, tamilNadu, "verified", nil}}, true, "native", false, "", "Coimbatore"},
		{"state scope other state", eventRow(evID, orgID, stPublished).rule("native", "state", 0, tamilNadu, true),
			[]ident{{1, "native", ernakulam, kerala, "verified", nil}}, false, "", false, "wrong_location", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, mock := deps(t, nil)
			ev := getAsPlayer(t, d, mock, c.rule, true, c.ids...)
			m := ev.MyEligibility
			if m.Eligible != c.eligible || m.Pending != c.pending || str(m.Reason) != c.reason {
				t.Fatalf("got %+v reason=%s", m, str(m.Reason))
			}
			if c.eligible && (str(m.Via) != c.via || !strings.Contains(str(m.Tag), c.tag)) {
				t.Fatalf("via=%s tag=%s", str(m.Via), str(m.Tag))
			}
			if !c.eligible && (m.Action == "" || m.Action == "none") {
				t.Fatalf("ineligible needs an action: %+v", m)
			}
			if c.eligible != (ev.PaymentInstructions != nil) {
				t.Fatalf("paymentInstructions visible=%v for eligible=%v", ev.PaymentInstructions != nil, c.eligible)
			}
		})
	}
}

func TestEligibilitySwitchOffMeansOpen(t *testing.T) {
	d, mock := deps(t, map[string]any{"location.enabled": false})
	r := eventRow(evID, orgID, stPublished).rule("native", "district", thanjavur, tamilNadu, true)
	ev := getAsPlayer(t, d, mock, r, false) // no identities query at all
	if !ev.MyEligibility.Eligible || str(ev.MyEligibility.Via) != "open" || ev.PaymentInstructions == nil {
		t.Fatalf("switch off: %+v", ev.MyEligibility)
	}
}

func TestOpenEventNeverLoadsIdentities(t *testing.T) {
	d, mock := deps(t, nil)
	ev := getAsPlayer(t, d, mock, eventRow(evID, orgID, stPublished), false)
	if !ev.MyEligibility.Eligible || ev.Eligibility.Summary != "Open to all players." {
		t.Fatalf("open: %+v %q", ev.MyEligibility, ev.Eligibility.Summary)
	}
}

// ---------- list ----------

func TestListAppliesFeedFilter(t *testing.T) {
	d, mock := deps(t, nil)
	expectIdentities(mock, playerID, ident{1, "native", thanjavur, tamilNadu, "verified", nil})
	r := eventRow(evID, orgID, stPublished).rule("native", "district", thanjavur, tamilNadu, true)
	mock.ExpectQuery(`WHERE e.status = 'published'.+e.eligibility_type = 'open'.+ORDER BY e.starts_at ASC, e.id ASC LIMIT \?`).
		WillReturnRows(evRows(false, r, eventRow(10, orgID, stPublished)))
	expectSummaries(mock, orgID)
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/events?limit=1", nil, nil)
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	e := decode(t, w.Body.Bytes())
	var items []Summary
	_ = json.Unmarshal(e.Data, &items)
	if len(items) != 1 || !items[0].MyEligibility.Eligible || e.Meta["nextCursor"] == "" || e.Meta["nextCursor"] == nil {
		t.Fatalf("items %+v meta %v", items, e.Meta)
	}
}

func TestListShowAllGreysIneligible(t *testing.T) {
	d, mock := deps(t, nil)
	r := eventRow(evID, orgID, stPublished).rule("native", "district", thanjavur, tamilNadu, true)
	mock.ExpectQuery(`WHERE e.status = 'published'`).WillReturnRows(evRows(false, r))
	expectSummaries(mock, orgID)
	expectIdentities(mock, playerID) // loaded lazily, once, for myEligibility only
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/events?showAll=true", nil, nil)
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	var items []Summary
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &items)
	if len(items) != 1 || items[0].MyEligibility.Eligible || items[0].MyEligibility.Action != "verify_native" {
		t.Fatalf("items %+v", items[0].MyEligibility)
	}
}

func TestListQueryFilterSQL(t *testing.T) {
	now := t0
	pg := httpx.PageParams{Limit: 20}
	sq, args := listQuery(playerID, ListFilter{}, pg, now, nil, "", "e.eligibility_type = 'open' OR x = ?", []any{int64(5)})
	if !strings.Contains(sq, "AND e.eligibility_type = 'open' OR x = ?") || args[len(args)-2] != int64(5) {
		t.Fatalf("filter not applied: %s %v", sq, args)
	}
	sq, _ = listQuery(playerID, ListFilter{ShowAll: true}, pg, now, nil, "", "", nil)
	if strings.Contains(sq, "AND e.eligibility_type") {
		t.Fatalf("showAll must not filter: %s", sq)
	}
	lat, lng := 10.78, 79.13
	sq, args = listQuery(playerID, ListFilter{Lat: &lat, Lng: &lng, RadiusKm: 50, SortDistance: true}, pg, now, nil, "", "", nil)
	if !strings.Contains(sq, "AS dist") || !strings.Contains(sq, "e.latitude IS NOT NULL") || !strings.Contains(sq, "ORDER BY dist ASC") {
		t.Fatalf("distance sql: %s", sq)
	}
	if strings.Contains(sq, "10.78") || args[0] != lat || args[2] != lng {
		t.Fatalf("coordinates must be bound: %v", args)
	}
	sq, _ = listQuery(playerID, ListFilter{Q: "50%_off"}, pg, now, nil, "", "", nil)
	if strings.Contains(sq, "50%") {
		t.Fatalf("search term in sql")
	}
}

func TestListRadiusValidation(t *testing.T) {
	d, _ := deps(t, map[string]any{"event.max_search_radius_km": 100})
	h := userEngine(d, testutil.VerifiedUser(playerID))
	for _, q := range []string{"lat=10&lng=79&radiusKm=0", "lat=10&lng=79&radiusKm=101", "radiusKm=10",
		"lat=10&radiusKm=10", "sort=distance", "sort=price", "lat=95&lng=79", "eventType=party", "fromDate=12-11-2026"} {
		w := testutil.Request(h, "GET", "/v1/events?"+q, nil, nil)
		if w.Code != 422 {
			t.Errorf("%s: code %d", q, w.Code)
		}
	}
}

func TestListDistanceRounded(t *testing.T) {
	d, mock := deps(t, nil)
	expectIdentities(mock, playerID)
	r := eventRow(evID, orgID, stPublished)
	r["latitude"], r["longitude"], r["dist"] = 10.8, 79.1, 12.36
	mock.ExpectQuery(`AS dist.+ORDER BY dist ASC`).WillReturnRows(evRows(true, r))
	expectSummaries(mock, orgID)
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/events?lat=10.78&lng=79.13&radiusKm=50&sort=distance", nil, nil)
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	var items []Summary
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &items)
	if len(items) != 1 || items[0].DistanceKm == nil || *items[0].DistanceKm != 12.4 {
		t.Fatalf("distance %+v", items)
	}
}

func TestFeaturedFiltersAndSize(t *testing.T) {
	d, mock := deps(t, map[string]any{"event.featured_list_size": 3})
	expectIdentities(mock, playerID)
	mock.ExpectQuery(`e.featured_at IS NOT NULL.+e.eligibility_type = 'open'.+LIMIT \?`).
		WithArgs(t0, playerID, playerID, int64(1), 3).WillReturnRows(evRows(false))
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/events/featured?sportId=1", nil, nil)
	if w.Code != 200 || string(decode(t, w.Body.Bytes()).Data) != "[]" {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestMineStatusValidation(t *testing.T) {
	d, _ := deps(t, nil)
	if w := testutil.Request(userEngine(d, organiser()), "GET", "/v1/me/events?status=bogus", nil, nil); w.Code != 422 {
		t.Fatalf("code %d", w.Code)
	}
}

func TestPreview(t *testing.T) {
	d, _ := deps(t, map[string]any{"event.eligibility_default_government": "either"})
	h := userEngine(d, organiser())
	w := testutil.Request(h, "GET", "/v1/events/eligibility/preview?category=government&type=native&scope=district&districtId=26", nil, nil)
	var p EligibilityPreview
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &p)
	if w.Code != 200 || !p.Valid || p.SuggestedType != "either" || !strings.Contains(p.Summary, "Thanjavur") {
		t.Fatalf("preview %d %+v", w.Code, p)
	}
	w = testutil.Request(h, "GET", "/v1/events/eligibility/preview?type=native", nil, nil)
	p = EligibilityPreview{}
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &p)
	if p.Valid || p.Fields["eligibility.scope"] == "" || p.SuggestedType != "open" {
		t.Fatalf("invalid preview %+v", p)
	}
}

// ---------- contact ----------

func expectPhone(t *testing.T, d *app.Deps, mock sqlmock.Sqlmock) {
	enc, err := d.Crypto.EncryptString("9876543210")
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT contact_phone_enc FROM events WHERE id = \?`).WithArgs(evID).
		WillReturnRows(sqlmock.NewRows([]string{"p"}).AddRow(enc))
}

func TestContactNeedsRegistration(t *testing.T) {
	d, mock := deps(t, nil)
	expectGet(mock, eventRow(evID, orgID, stPublished), false)
	expectBlocked(mock, playerID, orgID, 0)
	expectMyRegistration(mock, evID, playerID, "")
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/events/9/contact", nil, nil)
	if w.Code != 403 || decode(t, w.Body.Bytes()).Error.Code != httpx.CodeForbidden {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestContactNotEligible(t *testing.T) {
	d, mock := deps(t, map[string]any{"event.show_contact": "always"})
	expectGet(mock, eventRow(evID, orgID, stPublished).rule("native", "district", thanjavur, tamilNadu, true), false)
	expectBlocked(mock, playerID, orgID, 0)
	expectIdentities(mock, playerID, ident{1, "native", ernakulam, kerala, "verified", nil})
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/events/9/contact", nil, nil)
	e := decode(t, w.Body.Bytes())
	if w.Code != 403 || e.Error.Code != httpx.CodeNotEligible || e.Error.Fields["reason"] != "wrong_location" || e.Error.Fields["action"] == "" {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestContactRevealAndRateLimit(t *testing.T) {
	d, mock := deps(t, map[string]any{"event.contact_reveals_per_day": 1})
	h := userEngine(d, testutil.VerifiedUser(playerID))
	expectGet(mock, eventRow(evID, orgID, stPublished), false)
	expectBlocked(mock, playerID, orgID, 0)
	expectMyRegistration(mock, evID, playerID, "registered")
	expectPhone(t, d, mock)
	w := testutil.Request(h, "GET", "/v1/events/9/contact", nil, nil)
	var c Contact
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &c)
	if w.Code != 200 || c.ContactPhone != "+919876543210" {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	expectGet(mock, eventRow(evID, orgID, stPublished), false)
	expectBlocked(mock, playerID, orgID, 0)
	expectMyRegistration(mock, evID, playerID, "registered")
	if w := testutil.Request(h, "GET", "/v1/events/9/contact", nil, nil); w.Code != 429 {
		t.Fatalf("second reveal code %d", w.Code)
	}
}

func TestContactOwnerAlwaysAllowed(t *testing.T) {
	d, mock := deps(t, map[string]any{"event.contact_reveals_per_day": 0})
	expectGet(mock, eventRow(evID, orgID, stDraft), false)
	expectPhone(t, d, mock)
	if w := testutil.Request(userEngine(d, organiser()), "GET", "/v1/events/9/contact", nil, nil); w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

// ---------- cancel / feature ----------

func TestCancel(t *testing.T) {
	d, mock := deps(t, nil)
	got := capture(d)
	h := userEngine(d, organiser())
	if w := testutil.Request(h, "POST", "/v1/events/9/cancel", map[string]any{"reason": " "}, nil); w.Code != 422 {
		t.Fatalf("empty reason code %d", w.Code)
	}
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPublished), true)
	mock.ExpectExec(`UPDATE events SET status = \?, cancel_reason = \?, cancelled_at = \? WHERE id = \? AND status IN`).
		WithArgs(stCancelled, "Rain", t0, evID, stDraft, stPendingPayment, stPendingReview, stPublished).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectOwnerDetail(mock, eventRow(evID, orgID, stCancelled))
	w := testutil.Request(h, "POST", "/v1/events/9/cancel", map[string]any{"reason": "Rain"}, nil)
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	var p events.EventPayload
	if len(*got) != 1 || (*got)[0].Type != events.EventCancelled || (*got)[0].Decode(&p) != nil || p.Reason != "Rain" || p.EventID != evID {
		t.Fatalf("events %+v", *got)
	}
}

func TestCancelCompleted409(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stCompleted), true)
	mock.ExpectExec(`UPDATE events SET status`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if w := testutil.Request(userEngine(d, organiser()), "POST", "/v1/events/9/cancel", map[string]any{"reason": "x"}, nil); w.Code != 409 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func expectBenefit(mock sqlmock.Sqlmock, uid int64, val string) {
	rows := sqlmock.NewRows([]string{"k", "v"})
	if val != "" {
		rows.AddRow("featured_events", []byte(val))
	}
	mock.ExpectQuery(`FROM subscriptions s\s+JOIN premium_plans`).WithArgs(uid).WillReturnRows(rows)
}

func TestFeaturePlanRequired(t *testing.T) {
	d, mock := deps(t, nil)
	expectGet(mock, eventRow(evID, orgID, stPublished), false)
	expectBenefit(mock, orgID, "")
	w := testutil.Request(userEngine(d, organiser()), "POST", "/v1/events/9/feature", map[string]any{}, nil)
	e := decode(t, w.Body.Bytes())
	if w.Code != 403 || e.Error.Code != httpx.CodePlanRequired || e.Error.Fields["benefit"] != "featured_events" {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestFeatureMonthlyLimit(t *testing.T) {
	d, mock := deps(t, nil)
	expectGet(mock, eventRow(evID, orgID, stPublished), false)
	expectBenefit(mock, orgID, "2")
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPublished), true)
	// IST month of 2026-11-12: [2026-10-31T18:30Z, 2026-11-30T18:30Z)
	from := time.Date(2026, 10, 31, 18, 30, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM events WHERE organiser_id = \? AND featured_at >= \?`).
		WithArgs(orgID, from, from.AddDate(0, 0, 30)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(2))
	mock.ExpectRollback()
	w := testutil.Request(userEngine(d, organiser()), "POST", "/v1/events/9/feature", map[string]any{}, nil)
	if w.Code != 409 || decode(t, w.Body.Bytes()).Error.Code != httpx.CodeLimitReached {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestFeatureHappyAndIdempotent(t *testing.T) {
	d, mock := deps(t, nil)
	h := userEngine(d, organiser())
	expectGet(mock, eventRow(evID, orgID, stPublished), false)
	expectBenefit(mock, orgID, "2")
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPublished), true)
	mock.ExpectQuery(`featured_at >= \?`).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectExec(`UPDATE events SET featured_at = \? WHERE id = \? AND status = 'published' AND featured_at IS NULL`).
		WithArgs(t0, evID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	featured := eventRow(evID, orgID, stPublished)
	featured["featured_at"] = t0
	expectOwnerDetail(mock, featured)
	if w := testutil.Request(h, "POST", "/v1/events/9/feature", map[string]any{}, nil); w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	// Already featured: no entitlement use, no write.
	expectGet(mock, featured, false)
	expectSummaries(mock, orgID)
	expectGroupChat(mock, evID, 0)
	expectLatestListing(mock, evID)
	w := testutil.Request(h, "POST", "/v1/events/9/feature", map[string]any{}, nil)
	var ev Event
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &ev)
	if w.Code != 200 || !ev.IsFeatured {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

func TestFeatureDraft409(t *testing.T) {
	d, mock := deps(t, nil)
	expectGet(mock, eventRow(evID, orgID, stDraft), false)
	if w := testutil.Request(userEngine(d, organiser()), "POST", "/v1/events/9/feature", map[string]any{}, nil); w.Code != 409 {
		t.Fatalf("code %d", w.Code)
	}
}

// ---------- admin ----------

func expectAudit(mock sqlmock.Sqlmock, action string) {
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(adminID, action, "event", "9", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectAdminDetail(mock sqlmock.Sqlmock, r row) {
	expectGet(mock, r, false)
	expectSummaries(mock, orgID)
	expectGroupChat(mock, evID, 0)
	mock.ExpectQuery(`FROM listing_orders o LEFT JOIN listing_fee_rules r`).WithArgs(evID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "eid", "st", "fr", "rn", "min", "max", "ef", "ms", "bp", "gbp", "base", "gst", "tot", "prov", "poid", "ref", "exp", "paid"}).
			AddRow(int64(12), evID, "paid", int64(1), "Default", int64(9900), int64(499900), int64(50000), int64(16), int64(500), int64(1800), int64(40000), int64(7200), int64(47200), "mock", nil, int64(0), t0, t0))
}

func TestAdminNeedsPermission(t *testing.T) {
	d, _ := deps(t, nil)
	h := adminEngine(d, "users.read")
	for _, p := range []string{"/v1/admin/events", "/v1/admin/events/9"} {
		if w := testutil.Request(h, "GET", p, nil, nil); w.Code != 403 {
			t.Fatalf("%s code %d", p, w.Code)
		}
	}
	if w := testutil.Request(h, "POST", "/v1/admin/events/9/approve", map[string]any{}, nil); w.Code != 403 {
		t.Fatalf("approve code %d", w.Code)
	}
}

func TestAdminApprove(t *testing.T) {
	d, mock := deps(t, nil)
	got := capture(d)
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPendingReview), true)
	mock.ExpectExec(`UPDATE events SET status = \?, published_at = COALESCE\(published_at, \?\)`).
		WithArgs(stPublished, t0, evID, stPendingReview).WillReturnResult(sqlmock.NewResult(0, 1))
	expectAudit(mock, "event.approve")
	mock.ExpectCommit()
	expectAdminDetail(mock, eventRow(evID, orgID, stPublished))
	w := testutil.Request(adminEngine(d, "events.review"), "POST", "/v1/admin/events/9/approve", map[string]any{}, nil)
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	var ae AdminEvent
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &ae)
	if len(ae.ListingOrders) != 1 || ae.ListingOrders[0].Quote.GrossEntryPaise != 800000 || ae.MyEligibility != nil {
		t.Fatalf("admin event %+v", ae)
	}
	if len(*got) != 1 || (*got)[0].Type != events.EventPublished {
		t.Fatalf("events %+v", *got)
	}
}

func TestAdminApproveWrongState(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stDraft), true)
	mock.ExpectRollback()
	if w := testutil.Request(adminEngine(d, "events.review"), "POST", "/v1/admin/events/9/approve", map[string]any{}, nil); w.Code != 409 {
		t.Fatalf("code %d", w.Code)
	}
}

func TestAdminRejectAndRemove(t *testing.T) {
	d, mock := deps(t, nil)
	got := capture(d)
	h := adminEngine(d, "events.review")
	if w := testutil.Request(h, "POST", "/v1/admin/events/9/reject", map[string]any{}, nil); w.Code != 422 {
		t.Fatalf("reject without reason %d", w.Code)
	}
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPendingReview), true)
	mock.ExpectExec(`UPDATE events SET status = \?, rejection_reason = \?`).WithArgs(stRejected, "Bad venue", evID, stPendingReview).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectAudit(mock, "event.reject")
	mock.ExpectCommit()
	expectAdminDetail(mock, eventRow(evID, orgID, stRejected))
	if w := testutil.Request(h, "POST", "/v1/admin/events/9/reject", map[string]any{"reason": "Bad venue"}, nil); w.Code != 200 {
		t.Fatalf("reject %d %s", w.Code, w.Body)
	}
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPublished), true)
	mock.ExpectExec(`UPDATE events SET status = \?, cancel_reason = \?`).WithArgs(stRemoved, "Spam", t0, evID, stPublished).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectAudit(mock, "event.remove")
	mock.ExpectCommit()
	expectAdminDetail(mock, eventRow(evID, orgID, stRemoved))
	if w := testutil.Request(h, "POST", "/v1/admin/events/9/remove", map[string]any{"reason": "Spam"}, nil); w.Code != 200 {
		t.Fatalf("remove %d %s", w.Code, w.Body)
	}
	if len(*got) != 2 || (*got)[0].Type != events.EventRejected || (*got)[1].Type != events.EventCancelled {
		t.Fatalf("events %+v", *got)
	}
}

func TestAdminListWithListing(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectQuery(`WHERE 1=1 AND e.status = \? AND e.id = \? ORDER BY e.id DESC LIMIT \?`).WithArgs(stPendingReview, evID, 21).
		WillReturnRows(evRows(false, eventRow(evID, orgID, stPendingReview)))
	expectSummaries(mock, orgID)
	expectLatestListing(mock, evID)
	w := testutil.Request(adminEngine(d, "events.review"), "GET", "/v1/admin/events?status=pending_review&q=%239", nil, nil)
	var items []AdminSummary
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &items)
	if w.Code != 200 || len(items) != 1 || items[0].Listing == nil || items[0].Listing.TotalPaise != 47200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
}

// ---------- job ----------

func TestCompletePastIsIdempotent(t *testing.T) {
	d, mock := deps(t, nil)
	got := capture(d)
	rows := sqlmock.NewRows([]string{"id", "o", "s"}).AddRow(evID, orgID, int64(1)).AddRow(int64(10), orgID, int64(1))
	mock.ExpectQuery(`SELECT id, organiser_id, sport_id FROM events WHERE status = 'published' AND ends_at < \?`).
		WithArgs(t0, 500).WillReturnRows(rows)
	mock.ExpectExec(`UPDATE events SET status = 'completed'`).WithArgs(t0, evID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE events SET status = 'completed'`).WithArgs(t0, int64(10)).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := New(d).Jobs()["event.complete_past"](context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || (*got)[0].Type != events.EventCompleted {
		t.Fatalf("events %+v", *got)
	}
}

func TestAdminFeatureAndUnfeature(t *testing.T) {
	d, mock := deps(t, nil)
	h := adminEngine(d, "events.review")
	// feature a published event
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPublished), true)
	mock.ExpectExec(`UPDATE events SET featured_at = \? WHERE id = \? AND status = 'published' AND featured_at IS NULL`).
		WithArgs(t0, evID).WillReturnResult(sqlmock.NewResult(0, 1))
	expectAudit(mock, "event.feature")
	mock.ExpectCommit()
	featured := eventRow(evID, orgID, stPublished)
	featured["featured_at"] = t0
	expectAdminDetail(mock, featured)
	if w := testutil.Request(h, "POST", "/v1/admin/events/9/feature", map[string]any{}, nil); w.Code != 200 {
		t.Fatalf("feature %d %s", w.Code, w.Body)
	}
	// feature again: no-op, no audit
	mock.ExpectBegin()
	expectGet(mock, featured, true)
	mock.ExpectCommit()
	expectAdminDetail(mock, featured)
	if w := testutil.Request(h, "POST", "/v1/admin/events/9/feature", map[string]any{}, nil); w.Code != 200 {
		t.Fatalf("feature again %d %s", w.Code, w.Body)
	}
	// unfeature
	mock.ExpectBegin()
	expectGet(mock, featured, true)
	mock.ExpectExec(`UPDATE events SET featured_at = NULL WHERE id = \? AND featured_at IS NOT NULL`).
		WithArgs(evID).WillReturnResult(sqlmock.NewResult(0, 1))
	expectAudit(mock, "event.unfeature")
	mock.ExpectCommit()
	expectAdminDetail(mock, eventRow(evID, orgID, stPublished))
	if w := testutil.Request(h, "POST", "/v1/admin/events/9/unfeature", map[string]any{}, nil); w.Code != 200 {
		t.Fatalf("unfeature %d %s", w.Code, w.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminFeatureNotPublishedIs409(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectBegin()
	expectGet(mock, eventRow(evID, orgID, stPendingReview), true)
	mock.ExpectRollback()
	if w := testutil.Request(adminEngine(d, "events.review"), "POST", "/v1/admin/events/9/feature", map[string]any{}, nil); w.Code != 409 {
		t.Fatalf("code %d", w.Code)
	}
	if w := testutil.Request(adminEngine(d, "users.read"), "POST", "/v1/admin/events/9/unfeature", map[string]any{}, nil); w.Code != 403 {
		t.Fatalf("perm code %d", w.Code)
	}
}

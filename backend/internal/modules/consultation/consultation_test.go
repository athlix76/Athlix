package consultation

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/requirements"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

const (
	playerID int64 = 1
	coachID  int64 = 2
)

var t0 = time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)

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

func requireCoachPlan(d *app.Deps) {
	m := map[string]bool{}
	for k, v := range testutil.DefaultRequirements {
		m[k] = v
	}
	m["coach/coach_pro_subscription/offer_consultation"] = true
	d.Requirements = requirements.NewStatic(m)
}

func coachUser() userdir.Basic {
	u := testutil.VerifiedUser(coachID)
	u.PrimaryRole, u.Roles = "coach", []string{"coach"}
	return u
}

func engine(d *app.Deps, u userdir.Basic) http.Handler {
	r, routes := testutil.UserEngine(u)
	New(d).Routes(routes)
	return r
}

func expectBlocked(mock sqlmock.Sqlmock, a, b int64, n int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blocks`).WithArgs(a, b, b, a).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func expectUser(mock sqlmock.Sqlmock, id int64, status string, roles ...string) {
	mock.ExpectQuery(`SELECT id, display_name, primary_role, status, onboarding_step, verification_status, is_minor, guardian_consent_status FROM users WHERE id = \?`).
		WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id", "dn", "pr", "st", "ob", "vs", "minor", "g"}).
		AddRow(id, "Coach K", roles[0], status, "done", "verified", false, "not_required"))
	rr := sqlmock.NewRows([]string{"role"})
	for _, r := range roles {
		rr.AddRow(r)
	}
	mock.ExpectQuery(`SELECT role FROM user_roles WHERE user_id = \?`).WithArgs(id).WillReturnRows(rr)
}

func expectListing(mock sqlmock.Sqlmock, id int64, offered bool, modes string) {
	q := mock.ExpectQuery(`SELECT offered, description, fee_note, modes FROM coach_consultations WHERE user_id = \?`).WithArgs(id)
	if modes == "" {
		q.WillReturnRows(sqlmock.NewRows([]string{"o", "d", "f", "m"}))
		return
	}
	q.WillReturnRows(sqlmock.NewRows([]string{"o", "d", "f", "m"}).AddRow(offered, "Batting clinic", "Rs 500 per hour, paid to me", []byte(modes)))
}

func expectSummaries(mock sqlmock.Sqlmock, ids ...int64) {
	rows := sqlmock.NewRows([]string{"id", "dn", "pr", "vs", "photo", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"})
	args := []driver.Value{}
	for _, id := range ids {
		rows.AddRow(id, "User", "player", "verified", nil, nil, nil, nil, nil, nil, nil, nil, nil)
		args = append(args, id)
	}
	mock.ExpectQuery(`FROM users u\s+LEFT JOIN profiles`).WithArgs(args...).WillReturnRows(rows)
}

func expectBenefits(mock sqlmock.Sqlmock, uid int64, keys ...string) {
	rows := sqlmock.NewRows([]string{"k", "v"})
	for _, k := range keys {
		rows.AddRow(k, []byte("true"))
	}
	mock.ExpectQuery(`FROM subscriptions s\s+JOIN premium_plans`).WithArgs(uid).WillReturnRows(rows)
}

func expectPending(mock sqlmock.Sqlmock, player int64, forUpdate bool, coachIDs ...int64) {
	re := `SELECT id, coach_id FROM consultation_requests WHERE player_id = \? AND status = 'pending'`
	if forUpdate {
		re += ` FOR UPDATE`
	}
	rows := sqlmock.NewRows([]string{"id", "coach_id"})
	for i, c := range coachIDs {
		rows.AddRow(100+i, c)
	}
	mock.ExpectQuery(re).WithArgs(player).WillReturnRows(rows)
}

var reqCols = []string{"id", "coach_id", "player_id", "message", "preferred_mode", "status", "created_at", "updated_at"}

func reqRow(id int64, status string) *sqlmock.Rows {
	return sqlmock.NewRows(reqCols).AddRow(id, coachID, playerID, "Help with batting", "online", status, t0, t0)
}

// --- GET /coaches/:userId/consultation ---

func TestGetListingOther(t *testing.T) {
	d, mock := deps(t, nil)
	expectBlocked(mock, playerID, coachID, 0)
	expectUser(mock, coachID, "active", "coach")
	expectListing(mock, coachID, true, `["in_person","online"]`)
	expectSummaries(mock, coachID)
	expectPending(mock, playerID, false)
	w := testutil.Request(engine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/coaches/2/consultation", nil, nil)
	var l Listing
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &l)
	if w.Code != 200 || !l.Offered || !l.CanRequest || l.Coach.ID != coachID || len(l.Modes) != 2 {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestGetListingCanRequestFalseWhenPendingOrAtLimit(t *testing.T) {
	for name, tc := range map[string]struct {
		cfg     map[string]any
		pending []int64
	}{
		"pending with coach": {nil, []int64{coachID}},
		"at limit":           {map[string]any{"consultation.max_open_requests_per_player": 2}, []int64{7, 8}},
	} {
		d, mock := deps(t, tc.cfg)
		expectBlocked(mock, playerID, coachID, 0)
		expectUser(mock, coachID, "active", "coach")
		expectListing(mock, coachID, true, `["online"]`)
		expectSummaries(mock, coachID)
		expectPending(mock, playerID, false, tc.pending...)
		w := testutil.Request(engine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/coaches/2/consultation", nil, nil)
		var l Listing
		_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &l)
		if w.Code != 200 || l.CanRequest {
			t.Fatalf("%s: got %d %s", name, w.Code, w.Body)
		}
	}
}

func TestGetListingHidden(t *testing.T) {
	// Blocked either way.
	d, mock := deps(t, nil)
	expectBlocked(mock, playerID, coachID, 1)
	if w := testutil.Request(engine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/coaches/2/consultation", nil, nil); w.Code != 404 {
		t.Fatalf("blocked: %d", w.Code)
	}
	// Not a coach.
	d, mock = deps(t, nil)
	expectBlocked(mock, playerID, coachID, 0)
	expectUser(mock, coachID, "active", "player")
	if w := testutil.Request(engine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/coaches/2/consultation", nil, nil); w.Code != 404 {
		t.Fatalf("not coach: %d", w.Code)
	}
	// Not offered.
	d, mock = deps(t, nil)
	expectBlocked(mock, playerID, coachID, 0)
	expectUser(mock, coachID, "active", "coach")
	expectListing(mock, coachID, false, `["online"]`)
	if w := testutil.Request(engine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/coaches/2/consultation", nil, nil); w.Code != 404 {
		t.Fatalf("not offered: %d", w.Code)
	}
	// Plan required and the coach has no consultation_listing benefit.
	d, mock = deps(t, nil)
	requireCoachPlan(d)
	expectBlocked(mock, playerID, coachID, 0)
	expectUser(mock, coachID, "active", "coach")
	expectListing(mock, coachID, true, `["online"]`)
	expectBenefits(mock, coachID)
	if w := testutil.Request(engine(d, testutil.VerifiedUser(playerID)), "GET", "/v1/coaches/2/consultation", nil, nil); w.Code != 404 {
		t.Fatalf("no plan: %d", w.Code)
	}
}

func TestGetListingSelfWithoutRow(t *testing.T) {
	d, mock := deps(t, nil)
	expectUser(mock, coachID, "active", "coach")
	expectListing(mock, coachID, false, "")
	expectSummaries(mock, coachID)
	w := testutil.Request(engine(d, coachUser()), "GET", "/v1/coaches/2/consultation", nil, nil)
	var l Listing
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &l)
	if w.Code != 200 || l.Offered || l.CanRequest || l.Modes == nil {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

// --- PUT /me/consultation ---

func TestPutListingCoachOnly(t *testing.T) {
	d, _ := deps(t, nil)
	w := testutil.Request(engine(d, testutil.VerifiedUser(playerID)), "PUT", "/v1/me/consultation", map[string]any{"offered": true}, nil)
	if w.Code != 403 || decode(t, w.Body.Bytes()).Error.Code != "FORBIDDEN" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestPutListingPlanRequired(t *testing.T) {
	d, mock := deps(t, nil)
	requireCoachPlan(d)
	expectListing(mock, coachID, false, "")
	expectBenefits(mock, coachID)
	body := map[string]any{"offered": true, "modes": []string{"online"}}
	w := testutil.Request(engine(d, coachUser()), "PUT", "/v1/me/consultation", body, nil)
	e := decode(t, w.Body.Bytes())
	if w.Code != 403 || e.Error.Code != "PLAN_REQUIRED" || e.Error.Fields["benefit"] != "consultation_listing" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}

	// With the benefit the listing is saved.
	d, mock = deps(t, nil)
	requireCoachPlan(d)
	expectListing(mock, coachID, false, "")
	expectBenefits(mock, coachID, "consultation_listing")
	mock.ExpectExec(`INSERT INTO coach_consultations .* ON DUPLICATE KEY UPDATE`).
		WithArgs(coachID, true, nil, nil, `["online"]`).WillReturnResult(sqlmock.NewResult(0, 1))
	expectSummaries(mock, coachID)
	if w := testutil.Request(engine(d, coachUser()), "PUT", "/v1/me/consultation", body, nil); w.Code != 200 {
		t.Fatalf("entitled: %d %s", w.Code, w.Body)
	}
}

func TestPutListingNoPlanNeededByDefaultAndPartial(t *testing.T) {
	d, mock := deps(t, nil) // seed: coach_pro_subscription/offer_consultation = false
	expectListing(mock, coachID, false, `["online"]`)
	// description kept from the stored row, modes canonicalised and de-duplicated.
	mock.ExpectExec(`INSERT INTO coach_consultations`).
		WithArgs(coachID, true, "Batting clinic", "Free first session", `["online","in_person"]`).WillReturnResult(sqlmock.NewResult(0, 1))
	expectSummaries(mock, coachID)
	body := map[string]any{"offered": true, "feeNote": " Free first session ", "modes": []string{"in_person", "online", "online"}}
	w := testutil.Request(engine(d, coachUser()), "PUT", "/v1/me/consultation", body, nil)
	var l Listing
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &l)
	if w.Code != 200 || !l.Offered || *l.Description != "Batting clinic" || *l.FeeNote != "Free first session" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestPutListingValidation(t *testing.T) {
	for field, body := range map[string]map[string]any{
		"modes":       {"offered": true, "modes": []string{}},
		"description": {"description": string(make([]rune, 1001))},
		"feeNote":     {"feeNote": string(make([]rune, 201))},
	} {
		d, mock := deps(t, nil)
		expectListing(mock, coachID, false, "")
		w := testutil.Request(engine(d, coachUser()), "PUT", "/v1/me/consultation", body, nil)
		if w.Code != 422 || decode(t, w.Body.Bytes()).Error.Fields[field] == "" {
			t.Fatalf("%s: got %d %s", field, w.Code, w.Body)
		}
	}
	d, mock := deps(t, nil)
	expectListing(mock, coachID, false, "")
	w := testutil.Request(engine(d, coachUser()), "PUT", "/v1/me/consultation", map[string]any{"modes": []string{"phone"}}, nil)
	if w.Code != 422 {
		t.Fatalf("bad mode: %d", w.Code)
	}
}

// --- POST /coaches/:userId/consultation-requests ---

func expectBookable(mock sqlmock.Sqlmock) {
	expectBlocked(mock, playerID, coachID, 0)
	expectUser(mock, coachID, "active", "coach")
	expectListing(mock, coachID, true, `["online"]`)
}

var reqBody = map[string]any{"message": "Help with batting", "preferredMode": "online"}

func TestCreateRequestHappyPublishes(t *testing.T) {
	d, mock := deps(t, nil)
	var got events.ConsultationPayload
	d.Dispatcher.On(events.ConsultationRequested, func(_ context.Context, e events.Event) error { return e.Decode(&got) })
	expectBookable(mock)
	mock.ExpectBegin()
	expectPending(mock, playerID, true, 7, 8, 9, 10)
	mock.ExpectExec(`INSERT INTO consultation_requests`).WithArgs(coachID, playerID, "Help with batting", "online").
		WillReturnResult(sqlmock.NewResult(33, 1))
	mock.ExpectCommit()
	expectSummaries(mock, coachID, playerID)
	w := testutil.Request(engine(d, testutil.VerifiedUser(playerID)), "POST", "/v1/coaches/2/consultation-requests", reqBody, nil)
	var r Request
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &r)
	if w.Code != 200 || r.ID != 33 || r.Status != "pending" || r.Coach.ID != coachID || r.Player.ID != playerID {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
	if got.RequestID != 33 || got.CoachID != coachID || got.PlayerID != playerID || got.Status != "pending" {
		t.Fatalf("event %+v", got)
	}
}

func TestCreateRequestRules(t *testing.T) {
	h := func(d *app.Deps) http.Handler { return engine(d, testutil.VerifiedUser(playerID)) }

	d, _ := deps(t, nil)
	if w := testutil.Request(engine(d, coachUser()), "POST", "/v1/coaches/2/consultation-requests", reqBody, nil); w.Code != 422 {
		t.Fatalf("self: %d", w.Code)
	}
	if w := testutil.Request(h(d), "POST", "/v1/coaches/2/consultation-requests", map[string]any{"message": " ", "preferredMode": "phone"}, nil); w.Code != 422 {
		t.Fatalf("validation: %d", w.Code)
	}

	// Mode the coach does not offer.
	d, mock := deps(t, nil)
	expectBookable(mock)
	if w := testutil.Request(h(d), "POST", "/v1/coaches/2/consultation-requests", map[string]any{"message": "hi", "preferredMode": "in_person"}, nil); w.Code != 422 {
		t.Fatalf("mode: %d", w.Code)
	}

	// Duplicate pending with this coach.
	d, mock = deps(t, nil)
	expectBookable(mock)
	mock.ExpectBegin()
	expectPending(mock, playerID, true, coachID)
	mock.ExpectRollback()
	w := testutil.Request(h(d), "POST", "/v1/coaches/2/consultation-requests", reqBody, nil)
	if w.Code != 409 || decode(t, w.Body.Bytes()).Error.Code != "CONFLICT" {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body)
	}

	// Default limit 5 reached.
	d, mock = deps(t, nil)
	expectBookable(mock)
	mock.ExpectBegin()
	expectPending(mock, playerID, true, 7, 8, 9, 10, 11)
	mock.ExpectRollback()
	w = testutil.Request(h(d), "POST", "/v1/coaches/2/consultation-requests", reqBody, nil)
	if w.Code != 409 || decode(t, w.Body.Bytes()).Error.Code != "LIMIT_REACHED" {
		t.Fatalf("limit: %d %s", w.Code, w.Body)
	}

	// Raising the setting lets the same player through.
	d, mock = deps(t, map[string]any{"consultation.max_open_requests_per_player": 6})
	expectBookable(mock)
	mock.ExpectBegin()
	expectPending(mock, playerID, true, 7, 8, 9, 10, 11)
	mock.ExpectExec(`INSERT INTO consultation_requests`).WillReturnResult(sqlmock.NewResult(34, 1))
	mock.ExpectCommit()
	expectSummaries(mock, coachID, playerID)
	if w := testutil.Request(h(d), "POST", "/v1/coaches/2/consultation-requests", reqBody, nil); w.Code != 200 {
		t.Fatalf("raised limit: %d %s", w.Code, w.Body)
	}

	// Coach not offering → 404.
	d, mock = deps(t, nil)
	expectBlocked(mock, playerID, coachID, 0)
	expectUser(mock, coachID, "active", "coach")
	expectListing(mock, coachID, false, "")
	if w := testutil.Request(h(d), "POST", "/v1/coaches/2/consultation-requests", reqBody, nil); w.Code != 404 {
		t.Fatalf("not offered: %d", w.Code)
	}
}

// --- GET /me/consultation-requests ---

func TestListRequests(t *testing.T) {
	d, mock := deps(t, nil)
	h := engine(d, coachUser())
	if w := testutil.Request(h, "GET", "/v1/me/consultation-requests?as=admin", nil, nil); w.Code != 422 {
		t.Fatalf("bad as: %d", w.Code)
	}
	if w := testutil.Request(h, "GET", "/v1/me/consultation-requests?status=open", nil, nil); w.Code != 422 {
		t.Fatalf("bad status: %d", w.Code)
	}
	rows := sqlmock.NewRows(reqCols).
		AddRow(5, coachID, playerID, "a", "online", "pending", t0, t0).
		AddRow(4, coachID, 3, "b", "online", "pending", t0, t0)
	mock.ExpectQuery(`FROM consultation_requests WHERE coach_id = \? AND status = \? ORDER BY id DESC LIMIT \?`).
		WithArgs(coachID, "pending", 2).WillReturnRows(rows)
	expectSummaries(mock, coachID, playerID)
	w := testutil.Request(h, "GET", "/v1/me/consultation-requests?as=coach&status=pending&limit=1", nil, nil)
	e := decode(t, w.Body.Bytes())
	var out []Request
	_ = json.Unmarshal(e.Data, &out)
	if w.Code != 200 || len(out) != 1 || e.Meta["nextCursor"] == nil || out[0].CreatedAt.Format(time.RFC3339) != "2026-11-12T09:30:00Z" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

// --- respond / cancel / complete ---

func TestRespond(t *testing.T) {
	// Another coach cannot see the request: the query is scoped by coach_id.
	d, mock := deps(t, nil)
	other := coachUser()
	other.ID = 99
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM consultation_requests WHERE id = \? AND coach_id = \? FOR UPDATE`).WithArgs(int64(5), int64(99)).
		WillReturnRows(sqlmock.NewRows(reqCols))
	mock.ExpectRollback()
	if w := testutil.Request(engine(d, other), "POST", "/v1/consultation-requests/5/respond", map[string]any{"accept": true}, nil); w.Code != 404 {
		t.Fatalf("other coach: %d", w.Code)
	}

	d, mock = deps(t, nil)
	h := engine(d, coachUser())
	if w := testutil.Request(h, "POST", "/v1/consultation-requests/5/respond", map[string]any{}, nil); w.Code != 422 {
		t.Fatalf("missing accept: %d", w.Code)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE id = \? AND coach_id = \?`).WithArgs(int64(5), coachID).WillReturnRows(reqRow(5, "declined"))
	mock.ExpectRollback()
	w := testutil.Request(h, "POST", "/v1/consultation-requests/5/respond", map[string]any{"accept": true}, nil)
	if w.Code != 409 || decode(t, w.Body.Bytes()).Error.Code != "INVALID_STATE" {
		t.Fatalf("not pending: %d %s", w.Code, w.Body)
	}

	var got events.ConsultationPayload
	d.Dispatcher.On(events.ConsultationResponded, func(_ context.Context, e events.Event) error { return e.Decode(&got) })
	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE id = \? AND coach_id = \?`).WithArgs(int64(5), coachID).WillReturnRows(reqRow(5, "pending"))
	mock.ExpectExec(`UPDATE consultation_requests SET status = \? WHERE id = \? AND status = \?`).WithArgs("accepted", int64(5), "pending").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectSummaries(mock, coachID, playerID)
	w = testutil.Request(h, "POST", "/v1/consultation-requests/5/respond", map[string]any{"accept": true, "note": "See you Monday"}, nil)
	var r Request
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &r)
	if w.Code != 200 || r.Status != "accepted" || got.Status != "accepted" || got.RequestID != 5 {
		t.Fatalf("accept: %d %s %+v", w.Code, w.Body, got)
	}
}

func TestCancelAndComplete(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE id = \? AND player_id = \? FOR UPDATE`).WithArgs(int64(5), playerID).WillReturnRows(reqRow(5, "pending"))
	mock.ExpectExec(`UPDATE consultation_requests`).WithArgs("cancelled", int64(5), "pending").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectSummaries(mock, coachID, playerID)
	if w := testutil.Request(engine(d, testutil.VerifiedUser(playerID)), "POST", "/v1/consultation-requests/5/cancel", map[string]any{}, nil); w.Code != 200 {
		t.Fatalf("cancel: %d %s", w.Code, w.Body)
	}

	// Completing a pending request is not allowed.
	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE id = \? AND coach_id = \? FOR UPDATE`).WithArgs(int64(6), coachID).WillReturnRows(reqRow(6, "pending"))
	mock.ExpectRollback()
	w := testutil.Request(engine(d, coachUser()), "POST", "/v1/consultation-requests/6/complete", map[string]any{}, nil)
	if w.Code != 409 || decode(t, w.Body.Bytes()).Error.Code != "INVALID_STATE" {
		t.Fatalf("complete pending: %d %s", w.Code, w.Body)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE id = \? AND coach_id = \? FOR UPDATE`).WithArgs(int64(6), coachID).WillReturnRows(reqRow(6, "accepted"))
	mock.ExpectExec(`UPDATE consultation_requests`).WithArgs("completed", int64(6), "accepted").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectSummaries(mock, coachID, playerID)
	if w := testutil.Request(engine(d, coachUser()), "POST", "/v1/consultation-requests/6/complete", map[string]any{}, nil); w.Code != 200 {
		t.Fatalf("complete: %d %s", w.Code, w.Body)
	}

	// The coach cannot cancel on the player's behalf (scoped by player_id → 404).
	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE id = \? AND player_id = \? FOR UPDATE`).WithArgs(int64(5), coachID).WillReturnRows(sqlmock.NewRows(reqCols))
	mock.ExpectRollback()
	if w := testutil.Request(engine(d, coachUser()), "POST", "/v1/consultation-requests/5/cancel", map[string]any{}, nil); w.Code != 404 {
		t.Fatalf("coach cancel: %d", w.Code)
	}
}

func TestCleanModes(t *testing.T) {
	if m, ok := cleanModes([]string{"in_person", "online", "in_person"}); !ok || len(m) != 2 || m[0] != "online" {
		t.Fatalf("got %v %v", m, ok)
	}
	if _, ok := cleanModes([]string{"video"}); ok {
		t.Fatal("unknown mode accepted")
	}
}

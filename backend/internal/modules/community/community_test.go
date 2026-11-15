package community

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/requirements"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

// ---- helpers ----

var created = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

var commColumns = []string{"id", "title", "description", "avatar_upload_id", "owner_id", "s.id", "s.code", "s.name",
	"city", "state", "visibility", "join_policy", "member_count", "status", "created_at", "m.status", "m.role", "removal_reason"}

type cr struct {
	id, owner        int64
	vis, policy      string
	count            int
	status           string
	myStatus, myRole any
	sport            bool
	reason           any // conversations.removal_reason
}

func commRows(cs ...cr) *sqlmock.Rows {
	r := sqlmock.NewRows(commColumns)
	for _, c := range cs {
		vis, policy, status := c.vis, c.policy, c.status
		if vis == "" {
			vis = VisibilityPublic
		}
		if policy == "" {
			policy = JoinOpen
		}
		if status == "" {
			status = StatusActive
		}
		var sid, code, name any
		if c.sport {
			sid, code, name = int64(2), "football", "Football"
		}
		r.AddRow(c.id, "Pune Sunday Football", nil, nil, c.owner, sid, code, name, "Pune", nil, vis, policy, c.count, status, created, c.myStatus, c.myRole, c.reason)
	}
	return r
}

func summaryRows(ids ...int64) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"id", "display_name", "primary_role", "verification_status", "photo_upload_id", "city", "state", "s.id", "s.code", "s.name", "sp.id", "sp.code", "sp.name"})
	for _, id := range ids {
		r.AddRow(id, "Owner", "player", "verified", nil, nil, nil, nil, nil, nil, nil, nil, nil)
	}
	return r
}

// lockRows mocks repo.lock: owner, title, visibility, join_policy, member_count, status, my status, my role, blocked.
func lockRows(owner int64, vis, policy string, count int, myStatus, myRole string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"owner", "title", "visibility", "join_policy", "member_count", "status", "ms", "mr", "blocked"}).
		AddRow(owner, "Pune Sunday Football", vis, policy, count, StatusActive, myStatus, myRole, int64(0))
}

func userRows(id int64, minor bool) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "display_name", "primary_role", "status", "onboarding_step", "verification_status", "is_minor", "guardian_consent_status"}).
		AddRow(id, "U", "player", "active", "done", "verified", minor, "not_required")
}

func countRow(n int) *sqlmock.Rows { return sqlmock.NewRows([]string{"n"}).AddRow(n) }

const (
	qLock      = `FROM conversations c\s+LEFT JOIN conversation_members m .* FOR UPDATE OF c`
	qVisible   = `SELECT c\.id, c\.title, .* AND c\.id = \?`
	qSummaries = `FROM users u`
	qBenefits  = `FROM subscriptions s`
	qOwned     = `SELECT COUNT\(\*\) FROM conversations WHERE type = 'community' AND owner_id = \?`
	qSport     = `SELECT COUNT\(\*\) FROM sports WHERE id = \?`
	qMember    = `SELECT status, role FROM conversation_members`
	qUpsert    = `INSERT INTO conversation_members .* ON DUPLICATE KEY UPDATE`
	qCountUp   = `UPDATE conversations SET member_count = member_count \+ 1`
	qCountDown = `UPDATE conversations SET member_count = GREATEST`
)

type env struct {
	d    *app.Deps
	mock sqlmock.Sqlmock
	r    *gin.Engine
}

func newEnv(t *testing.T, u userdir.Basic, cfg map[string]any) env {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	r, routes := testutil.UserEngine(u)
	New(d).Routes(routes)
	return env{d: d, mock: mock, r: r}
}

func newAdminEnv(t *testing.T, perms []string) env {
	t.Helper()
	d, mock := testutil.Deps(t, nil)
	r, routes := testutil.AdminEngine(1, perms)
	New(d).Routes(routes)
	return env{d: d, mock: mock, r: r}
}

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Meta  map[string]any  `json:"meta"`
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func decode(t *testing.T, body []byte) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("bad json %s: %v", body, err)
	}
	return e
}

func wantCode(t *testing.T, w interface {
	Result() *http.Response
}, body []byte, status int, code string) envelope {
	t.Helper()
	if got := w.Result().StatusCode; got != status {
		t.Fatalf("status = %d, want %d; body %s", got, status, body)
	}
	e := decode(t, body)
	if code != "" && e.Error.Code != code {
		t.Fatalf("code = %q, want %q; body %s", e.Error.Code, code, body)
	}
	return e
}

type recorder struct {
	mu  sync.Mutex
	got []events.Event
}

func (r *recorder) on(d *app.Deps, typ string, check func()) {
	d.Dispatcher.On(typ, func(_ context.Context, e events.Event) error {
		if check != nil {
			check()
		}
		r.mu.Lock()
		r.got = append(r.got, e)
		r.mu.Unlock()
		return nil
	})
}

func minor(id int64) userdir.Basic {
	u := testutil.VerifiedUser(id)
	u.IsMinor = true
	return u
}

// ---- create ----

func TestCreateHappyPath(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(7), nil)
	e.mock.ExpectQuery(qBenefits).WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"k", "v"}))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qOwned).WithArgs(7).WillReturnRows(countRow(0))
	e.mock.ExpectQuery(qSport).WithArgs(2).WillReturnRows(countRow(1))
	e.mock.ExpectExec(`INSERT INTO conversations`).
		WithArgs("Pune Sunday Football", nil, nil, 7, 2, "Pune", "Maharashtra", "public", "approval").
		WillReturnResult(sqlmock.NewResult(51, 1))
	e.mock.ExpectExec(`INSERT INTO conversation_members .* 'owner', 'active'`).WithArgs(51, 7).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.mock.ExpectQuery(qVisible).WithArgs(7, 51, 7, 7).
		WillReturnRows(commRows(cr{id: 51, owner: 7, policy: JoinApproval, count: 1, myStatus: "active", myRole: "owner", sport: true}))
	e.mock.ExpectQuery(qSummaries).WithArgs(7).WillReturnRows(summaryRows(7))

	w := testutil.Request(e.r, "POST", "/v1/communities", map[string]any{
		"title": "  Pune Sunday Football ", "sportId": 2, "city": "Pune", "state": "Maharashtra",
		"visibility": "public", "joinPolicy": "approval", "description": "",
	}, nil)
	env := wantCode(t, w, w.Body.Bytes(), 200, "")
	var c map[string]any
	_ = json.Unmarshal(env.Data, &c)
	if c["id"].(float64) != 51 || c["myStatus"] != "active" || c["myRole"] != "owner" || c["joinPolicy"] != "approval" {
		t.Fatalf("unexpected community %v", c)
	}
	for _, k := range []string{"description", "avatarUrl", "state", "sport", "owner", "createdAt"} {
		if _, ok := c[k]; !ok {
			t.Fatalf("key %q missing (nulls must be present): %v", k, c)
		}
	}
	if s := c["sport"].(map[string]any); s["code"] != "football" {
		t.Fatalf("sport = %v", s)
	}
}

func TestCreateLimitFromSetting(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(7), map[string]any{"chat.max_communities_created_per_user": 1})
	e.mock.ExpectQuery(qBenefits).WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"k", "v"}))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qOwned).WithArgs(7).WillReturnRows(countRow(1))
	e.mock.ExpectRollback()
	w := testutil.Request(e.r, "POST", "/v1/communities", map[string]any{"title": "Runners"}, nil)
	wantCode(t, w, w.Body.Bytes(), 409, "LIMIT_REACHED")
}

func TestCreateLimitRaisedByEntitlement(t *testing.T) {
	// Setting allows 1, the extra_communities benefit allows 25: the larger value wins.
	e := newEnv(t, testutil.VerifiedUser(7), map[string]any{"chat.max_communities_created_per_user": 1})
	e.mock.ExpectQuery(qBenefits).WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"k", "v"}).AddRow("extra_communities", []byte("25")))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qOwned).WithArgs(7).WillReturnRows(countRow(1))
	e.mock.ExpectExec(`INSERT INTO conversations`).WillReturnResult(sqlmock.NewResult(52, 1))
	e.mock.ExpectExec(`INSERT INTO conversation_members`).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.mock.ExpectQuery(qVisible).WillReturnRows(commRows(cr{id: 52, owner: 7, count: 1, myStatus: "active", myRole: "owner"}))
	e.mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(7))
	w := testutil.Request(e.r, "POST", "/v1/communities", map[string]any{"title": "Runners"}, nil)
	wantCode(t, w, w.Body.Bytes(), 200, "")

	// Without the benefit the same count is refused.
	e2 := newEnv(t, testutil.VerifiedUser(7), map[string]any{"chat.max_communities_created_per_user": 1})
	e2.mock.ExpectQuery(qBenefits).WillReturnRows(sqlmock.NewRows([]string{"k", "v"}))
	e2.mock.ExpectBegin()
	e2.mock.ExpectQuery(qOwned).WillReturnRows(countRow(1))
	e2.mock.ExpectRollback()
	w = testutil.Request(e2.r, "POST", "/v1/communities", map[string]any{"title": "Runners"}, nil)
	wantCode(t, w, w.Body.Bytes(), 409, "LIMIT_REACHED")
}

func TestCreateValidation(t *testing.T) {
	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"missing title", map[string]any{}, "title"},
		{"short title", map[string]any{"title": " ab "}, "title"},
		{"long title", map[string]any{"title": strings.Repeat("x", 81)}, "title"},
		{"long description", map[string]any{"title": "Runners", "description": strings.Repeat("d", 501)}, "description"},
		{"bad visibility", map[string]any{"title": "Runners", "visibility": "secret"}, "visibility"},
		{"bad policy", map[string]any{"title": "Runners", "joinPolicy": "anyone"}, "joinPolicy"},
		{"private open", map[string]any{"title": "Runners", "visibility": "private", "joinPolicy": "open"}, "joinPolicy"},
		{"bad sport", map[string]any{"title": "Runners", "sportId": -1}, "sportId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, testutil.VerifiedUser(7), nil)
			w := testutil.Request(e.r, "POST", "/v1/communities", tc.body, nil)
			env := wantCode(t, w, w.Body.Bytes(), 422, "VALIDATION_FAILED")
			if env.Error.Fields[tc.field] == "" {
				t.Fatalf("fields = %v, want %q", env.Error.Fields, tc.field)
			}
		})
	}
}

func TestCreateUnknownSportAndForeignAvatar(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(7), nil)
	e.mock.ExpectQuery(qBenefits).WillReturnRows(sqlmock.NewRows([]string{"k", "v"}))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qOwned).WillReturnRows(countRow(0))
	e.mock.ExpectQuery(qSport).WithArgs(99).WillReturnRows(countRow(0))
	e.mock.ExpectRollback()
	w := testutil.Request(e.r, "POST", "/v1/communities", map[string]any{"title": "Runners", "sportId": 99}, nil)
	if env := wantCode(t, w, w.Body.Bytes(), 422, "VALIDATION_FAILED"); env.Error.Fields["sportId"] == "" {
		t.Fatalf("fields = %v", env.Error.Fields)
	}

	e2 := newEnv(t, testutil.VerifiedUser(7), nil)
	e2.mock.ExpectQuery(qBenefits).WillReturnRows(sqlmock.NewRows([]string{"k", "v"}))
	e2.mock.ExpectBegin()
	e2.mock.ExpectQuery(qOwned).WillReturnRows(countRow(0))
	e2.mock.ExpectQuery(`FROM media_uploads WHERE id = \? AND user_id = \?`).WithArgs(5, 7).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	e2.mock.ExpectRollback()
	w = testutil.Request(e2.r, "POST", "/v1/communities", map[string]any{"title": "Runners", "avatarUploadId": 5}, nil)
	if env := wantCode(t, w, w.Body.Bytes(), 422, "VALIDATION_FAILED"); env.Error.Fields["avatarUploadId"] == "" {
		t.Fatalf("fields = %v", env.Error.Fields)
	}
}

func TestCreateVerificationRequirement(t *testing.T) {
	e := newEnv(t, func() userdir.Basic { u := testutil.VerifiedUser(7); u.VerificationStatus = "unverified"; return u }(), nil)
	e.d.Requirements = requirements.NewStatic(map[string]bool{"player/digilocker/create_community": true})
	w := testutil.Request(e.r, "POST", "/v1/communities", map[string]any{"title": "Runners"}, nil)
	wantCode(t, w, w.Body.Bytes(), 403, "VERIFICATION_REQUIRED")
}

// ---- read ----

func TestGetPrivateIsNotFoundForOutsider(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(8), nil)
	e.mock.ExpectQuery(qVisible+`.*m\.status IN \('active','pending','invited'\).*blocks`).WithArgs(8, 51, 8, 8).WillReturnRows(sqlmock.NewRows(commColumns))
	w := testutil.Request(e.r, "GET", "/v1/communities/51", nil, nil)
	wantCode(t, w, w.Body.Bytes(), 404, "NOT_FOUND")
}

func TestGetBannedMemberSeesBanned(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(8), nil)
	e.mock.ExpectQuery(qVisible).WillReturnRows(commRows(cr{id: 51, owner: 7, count: 3, myStatus: "banned", myRole: "member"}))
	e.mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(7))
	w := testutil.Request(e.r, "GET", "/v1/communities/51", nil, nil)
	env := wantCode(t, w, w.Body.Bytes(), 200, "")
	var c Community
	_ = json.Unmarshal(env.Data, &c)
	if c.MyStatus != "banned" || c.MyRole != nil || c.Owner == nil || c.Owner.ID != 7 {
		t.Fatalf("community = %+v", c)
	}
}

func TestBrowseFiltersAndCursor(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(8), nil)
	e.mock.ExpectQuery(`c\.visibility = 'public'.*c\.title LIKE \? OR c\.description LIKE \?.*c\.sport_id = \?.*c\.state = \?.*ORDER BY c\.id DESC LIMIT \?`).
		WithArgs(8, 8, 8, `%50\%%`, `%50\%%`, 2, "Maharashtra", 2).
		WillReturnRows(commRows(cr{id: 60, owner: 7}, cr{id: 59, owner: 7}))
	e.mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(7))
	w := testutil.Request(e.r, "GET", "/v1/communities?q=50%25&sportId=2&state=Maharashtra&limit=1", nil, nil)
	env := wantCode(t, w, w.Body.Bytes(), 200, "")
	var list []Community
	_ = json.Unmarshal(env.Data, &list)
	if len(list) != 1 || list[0].ID != 60 || list[0].MyStatus != "none" || env.Meta["nextCursor"] == nil {
		t.Fatalf("list = %+v meta = %v", list, env.Meta)
	}
}

func TestMineEmptyIsArray(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(8), nil)
	e.mock.ExpectQuery(`m\.status IN \('active','pending','invited'\) ORDER BY c\.id DESC`).WithArgs(8, 21).WillReturnRows(sqlmock.NewRows(commColumns))
	w := testutil.Request(e.r, "GET", "/v1/me/communities", nil, nil)
	env := wantCode(t, w, w.Body.Bytes(), 200, "")
	if string(env.Data) != "[]" {
		t.Fatalf("data = %s", env.Data)
	}
}

// ---- update / delete ----

func TestUpdatePartial(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(7), nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WithArgs(7, 7, 7, 51).WillReturnRows(lockRows(7, "public", "open", 3, "active", "owner"))
	e.mock.ExpectQuery(`SELECT COALESCE\(title, ''\), description`).WithArgs(51).
		WillReturnRows(sqlmock.NewRows([]string{"t", "d", "a", "s", "c", "st", "v", "j"}).AddRow("Old", "Desc", nil, 2, "Pune", nil, "public", "open"))
	// description null clears it; untouched fields stay; sport unchanged → no sport query.
	e.mock.ExpectExec(`UPDATE conversations SET title = \?`).
		WithArgs("New name", nil, nil, 2, "Pune", nil, "public", "approval", 51).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.mock.ExpectQuery(qVisible).WillReturnRows(commRows(cr{id: 51, owner: 7, policy: JoinApproval, myStatus: "active", myRole: "owner"}))
	e.mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(7))
	w := testutil.Request(e.r, "PUT", "/v1/communities/51", map[string]any{"title": "New name", "description": nil, "joinPolicy": "approval"}, nil)
	wantCode(t, w, w.Body.Bytes(), 200, "")
}

func TestUpdatePrivateWithOpenPolicyRejected(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(7), nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 3, "active", "admin"))
	e.mock.ExpectQuery(`SELECT COALESCE\(title, ''\), description`).
		WillReturnRows(sqlmock.NewRows([]string{"t", "d", "a", "s", "c", "st", "v", "j"}).AddRow("Old", nil, nil, nil, nil, nil, "public", "open"))
	e.mock.ExpectRollback()
	w := testutil.Request(e.r, "PUT", "/v1/communities/51", map[string]any{"visibility": "private"}, nil)
	if env := wantCode(t, w, w.Body.Bytes(), 422, "VALIDATION_FAILED"); env.Error.Fields["joinPolicy"] == "" {
		t.Fatalf("fields = %v", env.Error.Fields)
	}
}

func TestUpdateByPlainMemberForbidden(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(8), nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 3, "active", "member"))
	e.mock.ExpectRollback()
	w := testutil.Request(e.r, "PUT", "/v1/communities/51", map[string]any{"title": "Hijack"}, nil)
	wantCode(t, w, w.Body.Bytes(), 403, "FORBIDDEN")
}

func TestDeleteOwnerOnly(t *testing.T) {
	// An admin (not owner) of the community cannot delete it.
	e := newEnv(t, testutil.VerifiedUser(8), nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 3, "active", "admin"))
	e.mock.ExpectRollback()
	w := testutil.Request(e.r, "DELETE", "/v1/communities/51", nil, nil)
	wantCode(t, w, w.Body.Bytes(), 403, "FORBIDDEN")

	// An outsider of a private community gets 404.
	e2 := newEnv(t, testutil.VerifiedUser(9), nil)
	e2.mock.ExpectBegin()
	e2.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "private", "invite", 3, "", ""))
	e2.mock.ExpectRollback()
	w = testutil.Request(e2.r, "DELETE", "/v1/communities/51", nil, nil)
	wantCode(t, w, w.Body.Bytes(), 404, "NOT_FOUND")

	// The owner archives it.
	e3 := newEnv(t, testutil.VerifiedUser(7), nil)
	e3.mock.ExpectBegin()
	e3.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 3, "active", "owner"))
	e3.mock.ExpectExec(`UPDATE conversations SET status = \?`).WithArgs("archived", 51).WillReturnResult(sqlmock.NewResult(0, 1))
	e3.mock.ExpectCommit()
	w = testutil.Request(e3.r, "DELETE", "/v1/communities/51", nil, nil)
	if w.Code != 204 {
		t.Fatalf("status = %d", w.Code)
	}
}

// ---- join ----

func TestJoinOpen(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(8), nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WithArgs(8, 8, 8, 51).WillReturnRows(lockRows(7, "public", "open", 5, "", ""))
	e.mock.ExpectExec(qUpsert).WithArgs(51, 8, "active").WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(qCountUp).WithArgs(51).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	w := testutil.Request(e.r, "POST", "/v1/communities/51/join", map[string]any{}, nil)
	env := wantCode(t, w, w.Body.Bytes(), 200, "")
	if string(env.Data) != `{"myStatus":"active"}` {
		t.Fatalf("data = %s", env.Data)
	}
}

func TestJoinAlreadyActiveIsIdempotent(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(8), nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "active", "member"))
	e.mock.ExpectCommit()
	w := testutil.Request(e.r, "POST", "/v1/communities/51/join", map[string]any{}, nil)
	wantCode(t, w, w.Body.Bytes(), 200, "")
}

func TestJoinApprovalPublishesRequestAfterCommit(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(8), nil)
	rec := &recorder{}
	rec.on(e.d, events.CommunityJoinRequested, func() {
		if err := e.mock.ExpectationsWereMet(); err != nil {
			t.Errorf("event published before commit: %v", err)
		}
	})
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "approval", 5, "", ""))
	e.mock.ExpectExec(qUpsert).WithArgs(51, 8, "pending").WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	w := testutil.Request(e.r, "POST", "/v1/communities/51/join", map[string]any{}, nil)
	env := wantCode(t, w, w.Body.Bytes(), 200, "")
	if string(env.Data) != `{"myStatus":"pending"}` {
		t.Fatalf("data = %s", env.Data)
	}
	if len(rec.got) != 1 {
		t.Fatalf("events = %d", len(rec.got))
	}
	var p events.CommunityJoinPayload
	_ = rec.got[0].Decode(&p)
	if p != (events.CommunityJoinPayload{ConversationID: 51, UserID: 8, OwnerID: 7}) {
		t.Fatalf("payload = %+v", p)
	}
}

func TestJoinPolicies(t *testing.T) {
	cases := []struct {
		name               string
		vis, policy, my    string
		status             int
		code               string
		upsert             string
		minorUser, noMinor bool
	}{
		{name: "invite only", vis: "public", policy: "invite", status: 403, code: "FORBIDDEN"},
		{name: "invited accepts on join", vis: "public", policy: "invite", my: "invited", status: 200, upsert: "active"},
		{name: "banned", vis: "public", policy: "open", my: "banned", status: 403, code: "FORBIDDEN"},
		{name: "private outsider", vis: "private", policy: "approval", status: 404, code: "NOT_FOUND"},
		{name: "minor blocked by setting", vis: "public", policy: "open", status: 403, code: "FORBIDDEN", minorUser: true, noMinor: true},
		{name: "minor allowed by default", vis: "public", policy: "open", status: 200, upsert: "active", minorUser: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := testutil.VerifiedUser(8)
			if tc.minorUser {
				u = minor(8)
			}
			cfg := map[string]any{}
			if tc.noMinor {
				cfg["minor.can_join_public_communities"] = false
			}
			e := newEnv(t, u, cfg)
			e.mock.ExpectBegin()
			e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, tc.vis, tc.policy, 5, tc.my, "member"))
			if tc.upsert != "" {
				e.mock.ExpectExec(qUpsert).WithArgs(51, 8, tc.upsert).WillReturnResult(sqlmock.NewResult(0, 1))
				e.mock.ExpectExec(qCountUp).WillReturnResult(sqlmock.NewResult(0, 1))
				e.mock.ExpectCommit()
			} else {
				e.mock.ExpectRollback()
			}
			w := testutil.Request(e.r, "POST", "/v1/communities/51/join", map[string]any{}, nil)
			wantCode(t, w, w.Body.Bytes(), tc.status, tc.code)
		})
	}
}

func TestJoinGroupMaxMembers(t *testing.T) {
	// chat.group_max_members = 5 and the community already has 5 → 409.
	e := newEnv(t, testutil.VerifiedUser(8), map[string]any{"chat.group_max_members": 5})
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "", ""))
	e.mock.ExpectRollback()
	w := testutil.Request(e.r, "POST", "/v1/communities/51/join", map[string]any{}, nil)
	wantCode(t, w, w.Body.Bytes(), 409, "LIMIT_REACHED")

	// Raising the limit lets the same join through.
	e2 := newEnv(t, testutil.VerifiedUser(8), map[string]any{"chat.group_max_members": 6})
	e2.mock.ExpectBegin()
	e2.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "", ""))
	e2.mock.ExpectExec(qUpsert).WillReturnResult(sqlmock.NewResult(0, 1))
	e2.mock.ExpectExec(qCountUp).WillReturnResult(sqlmock.NewResult(0, 1))
	e2.mock.ExpectCommit()
	w = testutil.Request(e2.r, "POST", "/v1/communities/51/join", map[string]any{}, nil)
	wantCode(t, w, w.Body.Bytes(), 200, "")
}

func TestAcceptInvite(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(8), nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "private", "invite", 5, "invited", "member"))
	e.mock.ExpectExec(qUpsert).WithArgs(51, 8, "active").WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(qCountUp).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	e.mock.ExpectQuery(qVisible).WillReturnRows(commRows(cr{id: 51, owner: 7, vis: "private", policy: "invite", count: 6, myStatus: "active", myRole: "member"}))
	e.mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(7))
	w := testutil.Request(e.r, "POST", "/v1/communities/51/invites/accept", map[string]any{}, nil)
	wantCode(t, w, w.Body.Bytes(), 200, "")

	// Without an invitation the private community stays hidden.
	e2 := newEnv(t, testutil.VerifiedUser(9), nil)
	e2.mock.ExpectBegin()
	e2.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "private", "invite", 5, "", ""))
	e2.mock.ExpectRollback()
	w = testutil.Request(e2.r, "POST", "/v1/communities/51/invites/accept", map[string]any{}, nil)
	wantCode(t, w, w.Body.Bytes(), 404, "NOT_FOUND")
}

// ---- join requests ----

func TestApprovePublishesAfterCommit(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(10), nil) // 10 is a community admin
	rec := &recorder{}
	rec.on(e.d, events.CommunityJoinApproved, func() {
		if err := e.mock.ExpectationsWereMet(); err != nil {
			t.Errorf("event published before commit: %v", err)
		}
	})
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WithArgs(10, 10, 10, 51).WillReturnRows(lockRows(7, "public", "approval", 5, "active", "admin"))
	e.mock.ExpectQuery(qMember).WithArgs(51, 8).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}).AddRow("pending", "member"))
	e.mock.ExpectExec(`UPDATE conversation_members SET status = \?, joined_at`).WithArgs("active", 51, 8).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(qCountUp).WithArgs(51).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	w := testutil.Request(e.r, "POST", "/v1/communities/51/join-requests/8/approve", nil, nil)
	if w.Code != 204 {
		t.Fatalf("status = %d body %s", w.Code, w.Body)
	}
	if len(rec.got) != 1 {
		t.Fatalf("events = %d", len(rec.got))
	}
	var p events.CommunityJoinPayload
	_ = rec.got[0].Decode(&p)
	if p != (events.CommunityJoinPayload{ConversationID: 51, UserID: 8, OwnerID: 7}) || rec.got[0].ActorID != 10 {
		t.Fatalf("event = %+v payload %+v", rec.got[0], p)
	}
}

func TestApproveRules(t *testing.T) {
	// Plain member of a public community → 403; nothing published.
	e := newEnv(t, testutil.VerifiedUser(9), nil)
	rec := &recorder{}
	rec.on(e.d, events.CommunityJoinApproved, nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "approval", 5, "active", "member"))
	e.mock.ExpectRollback()
	w := testutil.Request(e.r, "POST", "/v1/communities/51/join-requests/8/approve", nil, nil)
	wantCode(t, w, w.Body.Bytes(), 403, "FORBIDDEN")
	if len(rec.got) != 0 {
		t.Fatal("event published on failure")
	}

	// No pending request → 404.
	e2 := newEnv(t, testutil.VerifiedUser(7), nil)
	e2.mock.ExpectBegin()
	e2.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "approval", 5, "active", "owner"))
	e2.mock.ExpectQuery(qMember).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}))
	e2.mock.ExpectRollback()
	w = testutil.Request(e2.r, "POST", "/v1/communities/51/join-requests/8/approve", nil, nil)
	wantCode(t, w, w.Body.Bytes(), 404, "NOT_FOUND")

	// Full community → 409.
	e3 := newEnv(t, testutil.VerifiedUser(7), map[string]any{"chat.group_max_members": 5})
	e3.mock.ExpectBegin()
	e3.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "approval", 5, "active", "owner"))
	e3.mock.ExpectQuery(qMember).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}).AddRow("pending", "member"))
	e3.mock.ExpectRollback()
	w = testutil.Request(e3.r, "POST", "/v1/communities/51/join-requests/8/approve", nil, nil)
	wantCode(t, w, w.Body.Bytes(), 409, "LIMIT_REACHED")
}

func TestRejectAndListRequests(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(7), nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "approval", 5, "active", "owner"))
	e.mock.ExpectQuery(qMember).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}).AddRow("pending", "member"))
	e.mock.ExpectExec(`DELETE FROM conversation_members WHERE conversation_id = \? AND user_id = \? AND status = 'pending'`).WithArgs(51, 8).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
	w := testutil.Request(e.r, "POST", "/v1/communities/51/join-requests/8/reject", nil, nil)
	if w.Code != 204 {
		t.Fatalf("status = %d", w.Code)
	}

	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "approval", 5, "active", "owner"))
	e.mock.ExpectQuery(`SELECT user_id, joined_at FROM conversation_members .* ORDER BY joined_at, user_id LIMIT \?`).WithArgs(51, 2).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "joined_at"}).AddRow(8, created).AddRow(9, created))
	e.mock.ExpectCommit()
	e.mock.ExpectQuery(qSummaries).WithArgs(8).WillReturnRows(summaryRows(8))
	w = testutil.Request(e.r, "GET", "/v1/communities/51/join-requests?limit=1", nil, nil)
	env := wantCode(t, w, w.Body.Bytes(), 200, "")
	var list []JoinRequest
	_ = json.Unmarshal(env.Data, &list)
	if len(list) != 1 || list[0].User.ID != 8 || env.Meta["nextCursor"] == nil {
		t.Fatalf("list = %+v meta %v", list, env.Meta)
	}
}

// ---- invites ----

func TestInvitesWithSkipped(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(7), nil) // owner, adult; minor.dm_policy default mutual_follow
	rec := &recorder{}
	rec.on(e.d, EventInvited, func() {
		if err := e.mock.ExpectationsWereMet(); err != nil {
			t.Errorf("event published before commit: %v", err)
		}
	})
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "private", "invite", 5, "active", "owner"))
	e.mock.ExpectQuery(`SELECT user_id, status FROM conversation_members WHERE conversation_id = \? AND user_id IN \(\?,\?,\?,\?\)`).
		WithArgs(51, 3, 4, 5, 7).WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}).AddRow(5, "active"))
	// 3: adult, not blocked → invited.
	e.mock.ExpectQuery(`FROM users WHERE id = \?`).WithArgs(3).WillReturnRows(userRows(3, false))
	e.mock.ExpectQuery(`FROM user_roles`).WithArgs(3).WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("player"))
	e.mock.ExpectQuery(`FROM blocks`).WithArgs(7, 3, 3, 7).WillReturnRows(countRow(0))
	e.mock.ExpectExec(qUpsert).WithArgs(51, 3, "invited").WillReturnResult(sqlmock.NewResult(0, 1))
	// 4: minor, only followed one way → skipped by minor.dm_policy = mutual_follow.
	e.mock.ExpectQuery(`FROM users WHERE id = \?`).WithArgs(4).WillReturnRows(userRows(4, true))
	e.mock.ExpectQuery(`FROM user_roles`).WithArgs(4).WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("player"))
	e.mock.ExpectQuery(`FROM blocks`).WithArgs(7, 4, 4, 7).WillReturnRows(countRow(0))
	e.mock.ExpectQuery(`FROM follows`).WithArgs(7, 4, 4, 7).WillReturnRows(countRow(1))
	// 5: already active, 7: self → skipped without lookups.
	e.mock.ExpectCommit()
	w := testutil.Request(e.r, "POST", "/v1/communities/51/invites", map[string]any{"userIds": []int64{3, 4, 5, 7, 3}}, nil)
	env := wantCode(t, w, w.Body.Bytes(), 200, "")
	if string(env.Data) != `{"invited":[3],"skipped":[4,5,7]}` {
		t.Fatalf("data = %s", env.Data)
	}
	if len(rec.got) != 1 || rec.got[0].ActorID != 7 {
		t.Fatalf("events = %+v", rec.got)
	}
	var p InvitedPayload
	_ = rec.got[0].Decode(&p)
	if p.ConversationID != 51 || p.UserID != 3 || p.OwnerID != 7 || p.InviterID != 7 {
		t.Fatalf("payload = %+v", p)
	}
}

func TestReinviteEmitsNoEvent(t *testing.T) {
	// User 3 is already invited: listed as invited again, no upsert, no second event.
	e := newEnv(t, testutil.VerifiedUser(7), nil)
	rec := &recorder{}
	rec.on(e.d, EventInvited, nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "private", "invite", 5, "active", "owner"))
	e.mock.ExpectQuery(`SELECT user_id, status FROM conversation_members`).WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}).AddRow(3, "invited"))
	e.mock.ExpectQuery(`FROM users WHERE id = \?`).WithArgs(3).WillReturnRows(userRows(3, false))
	e.mock.ExpectQuery(`FROM user_roles`).WithArgs(3).WillReturnRows(sqlmock.NewRows([]string{"role"}))
	e.mock.ExpectQuery(`FROM blocks`).WillReturnRows(countRow(0))
	e.mock.ExpectCommit()
	w := testutil.Request(e.r, "POST", "/v1/communities/51/invites", map[string]any{"userIds": []int64{3}}, nil)
	env := wantCode(t, w, w.Body.Bytes(), 200, "")
	if string(env.Data) != `{"invited":[3],"skipped":[]}` || len(rec.got) != 0 {
		t.Fatalf("data = %s events %d", env.Data, len(rec.got))
	}
}

// ---- ownership transfer ----

const qMemberOf = `SELECT status, role FROM conversation_members WHERE conversation_id = \? AND user_id = \?`

func TestTransferOwnership(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(7), nil)
	rec := &recorder{}
	rec.on(e.d, EventOwnershipTransferred, func() {
		if err := e.mock.ExpectationsWereMet(); err != nil {
			t.Errorf("event published before commit: %v", err)
		}
	})
	e.mock.ExpectQuery(`FROM users WHERE id = \?`).WithArgs(10).WillReturnRows(userRows(10, false))
	e.mock.ExpectQuery(`FROM user_roles`).WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"role"}))
	e.mock.ExpectQuery(qBenefits).WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"k", "v"}))
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "active", "owner"))
	e.mock.ExpectQuery(qMemberOf).WithArgs(51, 10).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}).AddRow("active", "admin"))
	e.mock.ExpectQuery(qOwned).WithArgs(10).WillReturnRows(countRow(0))
	e.mock.ExpectExec(`UPDATE conversations SET owner_id = \? WHERE id = \? AND type = 'community' AND owner_id = \?`).WithArgs(10, 51, 7).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`UPDATE conversation_members SET role = 'owner'`).WithArgs(51, 10).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`UPDATE conversation_members SET role = 'admin'`).WithArgs(51, 7).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`INSERT INTO community_ownership_transfers \(conversation_id, from_user_id, to_user_id\)`).WithArgs(51, 7, 10).WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectCommit()
	w := testutil.Request(e.r, "POST", "/v1/communities/51/transfer-ownership", map[string]any{"userId": 10}, nil)
	if w.Code != 204 {
		t.Fatalf("status = %d body %s", w.Code, w.Body)
	}
	if len(rec.got) != 1 || rec.got[0].ActorID != 7 {
		t.Fatalf("events = %+v", rec.got)
	}
	var p OwnershipPayload
	_ = rec.got[0].Decode(&p)
	if p.ConversationID != 51 || p.UserID != 10 || p.OwnerID != 10 || p.PreviousOwnerID != 7 {
		t.Fatalf("payload = %+v", p)
	}
}

func TestTransferOwnershipRejections(t *testing.T) {
	cached := false // userdir caches the target after the first lookup; entitlements are read each time
	expectTarget := func(e env) {
		if !cached {
			e.mock.ExpectQuery(`FROM users WHERE id = \?`).WithArgs(10).WillReturnRows(userRows(10, false))
			e.mock.ExpectQuery(`FROM user_roles`).WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"role"}))
			cached = true
		}
		e.mock.ExpectQuery(qBenefits).WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"k", "v"}))
	}
	e := newEnv(t, testutil.VerifiedUser(7), map[string]any{"chat.max_communities_created_per_user": 2})
	rec := &recorder{}
	rec.on(e.d, EventOwnershipTransferred, nil)

	// Missing userId → 422; self → 409 INVALID_STATE.
	w := testutil.Request(e.r, "POST", "/v1/communities/51/transfer-ownership", map[string]any{}, nil)
	wantCode(t, w, w.Body.Bytes(), 422, "VALIDATION_FAILED")
	w = testutil.Request(e.r, "POST", "/v1/communities/51/transfer-ownership", map[string]any{"userId": 7}, nil)
	wantCode(t, w, w.Body.Bytes(), 409, "INVALID_STATE")

	// A community admin (not owner) → 403.
	expectTarget(e)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "active", "admin"))
	e.mock.ExpectRollback()
	w = testutil.Request(e.r, "POST", "/v1/communities/51/transfer-ownership", map[string]any{"userId": 10}, nil)
	wantCode(t, w, w.Body.Bytes(), 403, "FORBIDDEN")

	// Target is a plain member → 409 INVALID_STATE.
	expectTarget(e)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "active", "owner"))
	e.mock.ExpectQuery(qMemberOf).WithArgs(51, 10).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}).AddRow("active", "member"))
	e.mock.ExpectRollback()
	w = testutil.Request(e.r, "POST", "/v1/communities/51/transfer-ownership", map[string]any{"userId": 10}, nil)
	wantCode(t, w, w.Body.Bytes(), 409, "INVALID_STATE")

	// Target not a member (or banned) → 404.
	expectTarget(e)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "active", "owner"))
	e.mock.ExpectQuery(qMemberOf).WithArgs(51, 10).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}).AddRow("banned", "member"))
	e.mock.ExpectRollback()
	w = testutil.Request(e.r, "POST", "/v1/communities/51/transfer-ownership", map[string]any{"userId": 10}, nil)
	wantCode(t, w, w.Body.Bytes(), 404, "NOT_FOUND")

	// Target already owns the setting's maximum → 409 LIMIT_REACHED.
	expectTarget(e)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "active", "owner"))
	e.mock.ExpectQuery(qMemberOf).WithArgs(51, 10).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}).AddRow("active", "admin"))
	e.mock.ExpectQuery(qOwned).WithArgs(10).WillReturnRows(countRow(2))
	e.mock.ExpectRollback()
	w = testutil.Request(e.r, "POST", "/v1/communities/51/transfer-ownership", map[string]any{"userId": 10}, nil)
	wantCode(t, w, w.Body.Bytes(), 409, "LIMIT_REACHED")

	// Unknown user → 404 before any lock.
	e.mock.ExpectQuery(`FROM users WHERE id = \?`).WithArgs(99).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	w = testutil.Request(e.r, "POST", "/v1/communities/51/transfer-ownership", map[string]any{"userId": 99}, nil)
	wantCode(t, w, w.Body.Bytes(), 404, "NOT_FOUND")

	if len(rec.got) != 0 {
		t.Fatal("event published on failure")
	}
}

func TestInviteMinorPolicyAnyoneAndNone(t *testing.T) {
	for _, tc := range []struct {
		policy string
		want   string
	}{{"anyone", `{"invited":[4],"skipped":[]}`}, {"none", `{"invited":[],"skipped":[4]}`}} {
		t.Run(tc.policy, func(t *testing.T) {
			e := newEnv(t, testutil.VerifiedUser(7), map[string]any{"minor.dm_policy": tc.policy})
			e.mock.ExpectBegin()
			e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "private", "invite", 5, "active", "owner"))
			e.mock.ExpectQuery(`SELECT user_id, status FROM conversation_members`).WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}))
			e.mock.ExpectQuery(`FROM users WHERE id = \?`).WithArgs(4).WillReturnRows(userRows(4, true))
			e.mock.ExpectQuery(`FROM user_roles`).WillReturnRows(sqlmock.NewRows([]string{"role"}))
			e.mock.ExpectQuery(`FROM blocks`).WillReturnRows(countRow(0))
			if tc.policy == "anyone" {
				e.mock.ExpectExec(qUpsert).WithArgs(51, 4, "invited").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			e.mock.ExpectCommit()
			w := testutil.Request(e.r, "POST", "/v1/communities/51/invites", map[string]any{"userIds": []int64{4}}, nil)
			env := wantCode(t, w, w.Body.Bytes(), 200, "")
			if string(env.Data) != tc.want {
				t.Fatalf("data = %s", env.Data)
			}
		})
	}
}

func TestInviteValidationAndPermission(t *testing.T) {
	e := newEnv(t, testutil.VerifiedUser(7), nil)
	w := testutil.Request(e.r, "POST", "/v1/communities/51/invites", map[string]any{"userIds": []int64{}}, nil)
	wantCode(t, w, w.Body.Bytes(), 422, "VALIDATION_FAILED")
	ids := make([]int64, 51)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	w = testutil.Request(e.r, "POST", "/v1/communities/51/invites", map[string]any{"userIds": ids}, nil)
	wantCode(t, w, w.Body.Bytes(), 422, "VALIDATION_FAILED")

	// A plain member cannot invite.
	e2 := newEnv(t, testutil.VerifiedUser(9), nil)
	e2.mock.ExpectBegin()
	e2.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "invite", 5, "active", "member"))
	e2.mock.ExpectRollback()
	w = testutil.Request(e2.r, "POST", "/v1/communities/51/invites", map[string]any{"userIds": []int64{3}}, nil)
	wantCode(t, w, w.Body.Bytes(), 403, "FORBIDDEN")
}

// ---- members ----

func TestSetRoleOwnerOnly(t *testing.T) {
	// A community admin cannot change roles.
	e := newEnv(t, testutil.VerifiedUser(10), nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "active", "admin"))
	e.mock.ExpectRollback()
	w := testutil.Request(e.r, "PUT", "/v1/communities/51/members/8/role", map[string]any{"role": "admin"}, nil)
	wantCode(t, w, w.Body.Bytes(), 403, "FORBIDDEN")

	// Outsider of a private community → 404.
	e2 := newEnv(t, testutil.VerifiedUser(11), nil)
	e2.mock.ExpectBegin()
	e2.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "private", "invite", 5, "", ""))
	e2.mock.ExpectRollback()
	w = testutil.Request(e2.r, "PUT", "/v1/communities/51/members/8/role", map[string]any{"role": "admin"}, nil)
	wantCode(t, w, w.Body.Bytes(), 404, "NOT_FOUND")

	// Bad role → 422 before any DB work.
	w = testutil.Request(e2.r, "PUT", "/v1/communities/51/members/8/role", map[string]any{"role": "owner"}, nil)
	wantCode(t, w, w.Body.Bytes(), 422, "VALIDATION_FAILED")

	// Owner promotes an active member.
	e3 := newEnv(t, testutil.VerifiedUser(7), nil)
	e3.mock.ExpectBegin()
	e3.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "active", "owner"))
	e3.mock.ExpectQuery(qMember).WithArgs(51, 8).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}).AddRow("active", "member"))
	e3.mock.ExpectExec(`UPDATE conversation_members SET role = \?`).WithArgs("admin", 51, 8).WillReturnResult(sqlmock.NewResult(0, 1))
	e3.mock.ExpectCommit()
	w = testutil.Request(e3.r, "PUT", "/v1/communities/51/members/8/role", map[string]any{"role": "admin"}, nil)
	if w.Code != 204 {
		t.Fatalf("status = %d body %s", w.Code, w.Body)
	}
}

func TestRemoveAndBan(t *testing.T) {
	// Admin cannot remove another admin.
	e := newEnv(t, testutil.VerifiedUser(10), nil)
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "active", "admin"))
	e.mock.ExpectQuery(qMember).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}).AddRow("active", "admin"))
	e.mock.ExpectRollback()
	w := testutil.Request(e.r, "POST", "/v1/communities/51/members/12/remove", nil, nil)
	wantCode(t, w, w.Body.Bytes(), 403, "FORBIDDEN")

	// Nobody acts on themselves here.
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "active", "admin"))
	e.mock.ExpectRollback()
	w = testutil.Request(e.r, "POST", "/v1/communities/51/members/10/remove", nil, nil)
	wantCode(t, w, w.Body.Bytes(), 409, "INVALID_STATE")

	// Owner removes an active member → member_count - 1.
	e2 := newEnv(t, testutil.VerifiedUser(7), nil)
	e2.mock.ExpectBegin()
	e2.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 5, "active", "owner"))
	e2.mock.ExpectQuery(qMember).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}).AddRow("active", "admin"))
	e2.mock.ExpectExec(`UPDATE conversation_members SET status = \? WHERE`).WithArgs("removed", 51, 12).WillReturnResult(sqlmock.NewResult(0, 1))
	e2.mock.ExpectExec(qCountDown).WithArgs(51).WillReturnResult(sqlmock.NewResult(0, 1))
	e2.mock.ExpectCommit()
	w = testutil.Request(e2.r, "POST", "/v1/communities/51/members/12/remove", nil, nil)
	if w.Code != 204 {
		t.Fatalf("status = %d body %s", w.Code, w.Body)
	}

	// Ban a non-member: row inserted as banned, count unchanged.
	e2.mock.ExpectBegin()
	e2.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 4, "active", "owner"))
	e2.mock.ExpectQuery(qMember).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}))
	e2.mock.ExpectExec(`INSERT INTO conversation_members .* 'banned'`).WithArgs(51, 13).WillReturnResult(sqlmock.NewResult(0, 1))
	e2.mock.ExpectCommit()
	w = testutil.Request(e2.r, "POST", "/v1/communities/51/members/13/ban", nil, nil)
	if w.Code != 204 {
		t.Fatalf("status = %d body %s", w.Code, w.Body)
	}

	// Removing someone with no membership → 404.
	e2.mock.ExpectBegin()
	e2.mock.ExpectQuery(qLock).WillReturnRows(lockRows(7, "public", "open", 4, "active", "owner"))
	e2.mock.ExpectQuery(qMember).WillReturnRows(sqlmock.NewRows([]string{"status", "role"}))
	e2.mock.ExpectRollback()
	w = testutil.Request(e2.r, "POST", "/v1/communities/51/members/14/remove", nil, nil)
	wantCode(t, w, w.Body.Bytes(), 404, "NOT_FOUND")
}

// ---- admin ----

func TestAdminListAndPermission(t *testing.T) {
	e := newAdminEnv(t, []string{"communities.moderate"})
	e.mock.ExpectQuery(`WHERE c\.type = 'community' AND c\.status = \? ORDER BY c\.id DESC LIMIT \?`).WithArgs(0, "removed", 21).
		WillReturnRows(commRows(cr{id: 51, owner: 7, status: "removed", reason: "spam"}))
	e.mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(7))
	w := testutil.Request(e.r, "GET", "/v1/admin/communities?status=removed", nil, nil)
	env := wantCode(t, w, w.Body.Bytes(), 200, "")
	var list []map[string]any
	_ = json.Unmarshal(env.Data, &list)
	if len(list) != 1 || list[0]["status"] != "removed" || list[0]["myStatus"] != "none" || list[0]["removalReason"] != "spam" {
		t.Fatalf("list = %v", list)
	}

	w = testutil.Request(e.r, "GET", "/v1/admin/communities?status=bogus", nil, nil)
	wantCode(t, w, w.Body.Bytes(), 422, "VALIDATION_FAILED")

	e2 := newAdminEnv(t, []string{"reports.read"})
	w = testutil.Request(e2.r, "GET", "/v1/admin/communities", nil, nil)
	wantCode(t, w, w.Body.Bytes(), 403, "FORBIDDEN")
	w = testutil.Request(e2.r, "POST", "/v1/admin/communities/51/remove", map[string]any{"reason": "spam"}, nil)
	wantCode(t, w, w.Body.Bytes(), 403, "FORBIDDEN")
}

func TestAdminRemoveAudited(t *testing.T) {
	e := newAdminEnv(t, []string{"communities.moderate"})
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(`SELECT status FROM conversations WHERE id = \? AND type = 'community' FOR UPDATE`).WithArgs(51).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	e.mock.ExpectExec(`UPDATE conversations SET status = 'removed', removal_reason = \? WHERE id = \? AND type = 'community'`).WithArgs("spam", 51).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(1, "community.remove", "conversation", "51", `{"previousStatus":"active","reasonLength":4}`, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	e.mock.ExpectCommit()
	w := testutil.Request(e.r, "POST", "/v1/admin/communities/51/remove", map[string]any{"reason": "spam"}, nil)
	if w.Code != 204 {
		t.Fatalf("status = %d body %s", w.Code, w.Body)
	}

	// Missing reason → 422; unknown id → 404.
	w = testutil.Request(e.r, "POST", "/v1/admin/communities/51/remove", map[string]any{"reason": " "}, nil)
	wantCode(t, w, w.Body.Bytes(), 422, "VALIDATION_FAILED")
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(`SELECT status FROM conversations`).WithArgs(99).WillReturnRows(sqlmock.NewRows([]string{"status"}))
	e.mock.ExpectRollback()
	w = testutil.Request(e.r, "POST", "/v1/admin/communities/99/remove", map[string]any{"reason": "spam"}, nil)
	wantCode(t, w, w.Body.Bytes(), 404, "NOT_FOUND")
}

func TestOptNullAndMissing(t *testing.T) {
	var in Input
	if err := json.Unmarshal([]byte(`{"description": null, "title": "x"}`), &in); err != nil {
		t.Fatal(err)
	}
	if !in.Description.Set || !in.Description.Null || !in.Title.Set || in.Title.V != "x" || in.City.Set {
		t.Fatalf("opt = %+v", in)
	}
}

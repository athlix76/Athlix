package follow

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

const (
	me    = int64(7)
	other = int64(9)
)

func q(s string) string { return regexp.QuoteMeta(s) }

type body struct {
	Data  json.RawMessage `json:"data"`
	Meta  map[string]any  `json:"meta"`
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func status(t *testing.T, w *httptest.ResponseRecorder, want int) body {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body)
	}
	var b body
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

type env struct {
	d       *app.Deps
	mock    sqlmock.Sqlmock
	m       *Module
	r       *gin.Engine
	created []events.FollowPayload
}

func newEnv(t *testing.T) *env {
	t.Helper()
	d, mock := testutil.Deps(t, nil)
	e := &env{d: d, mock: mock, m: New(d)}
	d.Dispatcher.On(events.FollowCreated, func(_ context.Context, ev events.Event) error {
		var p events.FollowPayload
		_ = ev.Decode(&p)
		e.created = append(e.created, p)
		return nil
	})
	r, routes := testutil.UserEngine(testutil.VerifiedUser(me))
	e.m.Routes(routes)
	e.r = r
	return e
}

func (e *env) do(method, path string) *httptest.ResponseRecorder {
	return testutil.Request(e.r, method, "/v1"+path, nil, nil)
}

func usr(id int64) userdir.Basic { return testutil.VerifiedUser(id) }

func (e *env) expectUser(b userdir.Basic) {
	e.mock.ExpectQuery(q(`SELECT id, display_name, primary_role, status, onboarding_step, verification_status, is_minor, guardian_consent_status FROM users WHERE id = ?`)).
		WithArgs(b.ID).WillReturnRows(sqlmock.NewRows([]string{"id", "display_name", "primary_role", "status", "onboarding_step", "verification_status", "is_minor", "guardian_consent_status"}).
		AddRow(b.ID, b.DisplayName, b.PrimaryRole, b.Status, b.OnboardingStep, b.VerificationStatus, b.IsMinor, b.GuardianConsentStatus))
	rows := sqlmock.NewRows([]string{"role"})
	for _, r := range b.Roles {
		rows.AddRow(r)
	}
	e.mock.ExpectQuery(q(`SELECT role FROM user_roles WHERE user_id = ?`)).WithArgs(b.ID).WillReturnRows(rows)
}

func (e *env) expectNoUser(id int64) {
	e.mock.ExpectQuery(q(`SELECT id, display_name, primary_role, status`)).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

func (e *env) expectBlocked(blocked bool) {
	n := 0
	if blocked {
		n = 1
	}
	e.mock.ExpectQuery(q(`SELECT COUNT(*) FROM blocks WHERE (blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?)`)).
		WithArgs(me, other, other, me).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func (e *env) expectFollows(follower, followee int64, yes bool) {
	n := 0
	if yes {
		n = 1
	}
	e.mock.ExpectQuery(q(`SELECT COUNT(*) FROM follows WHERE follower_id = ? AND followee_id = ?`)).WithArgs(follower, followee).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func (e *env) expectAdjust(col string, uid int64, delta int) {
	query := `UPDATE profiles SET ` + col + ` = ` + col + ` + 1 WHERE user_id = ?`
	if delta < 0 {
		query = `UPDATE profiles SET ` + col + ` = ` + col + ` - 1 WHERE user_id = ? AND ` + col + ` > 0`
	}
	e.mock.ExpectExec(q(query)).WithArgs(uid).WillReturnResult(sqlmock.NewResult(0, 1))
}

func (e *env) expectCount(uid int64, n int) {
	e.mock.ExpectQuery(q(`SELECT followers_count FROM profiles WHERE user_id = ?`)).WithArgs(uid).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func (e *env) expectInsertFollow(added bool) {
	n := int64(0)
	if added {
		n = 1
	}
	e.mock.ExpectExec(q(`INSERT IGNORE INTO follows (follower_id, followee_id) SELECT ?, ? FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM blocks`)).
		WithArgs(me, other, me, other, other, me).WillReturnResult(sqlmock.NewResult(0, n))
}

func (e *env) expectDeleteFollow(follower, followee int64, removed bool) {
	n := int64(0)
	if removed {
		n = 1
	}
	e.mock.ExpectExec(q(`DELETE FROM follows WHERE follower_id = ? AND followee_id = ?`)).WithArgs(follower, followee).WillReturnResult(sqlmock.NewResult(0, n))
}

func state(t *testing.T, b body) State {
	t.Helper()
	var s State
	if err := json.Unmarshal(b.Data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// ---- follow / unfollow ----

func TestFollowHappyPathAndIdempotent(t *testing.T) {
	e := newEnv(t)
	e.expectUser(usr(other))
	e.expectBlocked(false)
	e.mock.ExpectBegin()
	e.expectInsertFollow(true)
	e.expectAdjust("followers_count", other, 1)
	e.expectAdjust("following_count", me, 1)
	e.expectCount(other, 4)
	e.mock.ExpectCommit()
	if s := state(t, status(t, e.do(http.MethodPost, "/users/9/follow"), http.StatusOK)); s != (State{Following: true, FollowersCount: 4}) {
		t.Fatalf("state = %+v", s)
	}
	if len(e.created) != 1 || e.created[0] != (events.FollowPayload{FollowerID: me, FolloweeID: other}) {
		t.Fatalf("events = %+v", e.created)
	}

	// Again: nothing inserted, no counter change, no second event (the user is cached).
	e.expectBlocked(false)
	e.mock.ExpectBegin()
	e.expectInsertFollow(false)
	e.expectCount(other, 4)
	e.mock.ExpectCommit()
	e.expectFollows(me, other, true)
	if s := state(t, status(t, e.do(http.MethodPost, "/users/9/follow"), http.StatusOK)); s != (State{Following: true, FollowersCount: 4}) {
		t.Fatalf("state = %+v", s)
	}
	if len(e.created) != 1 {
		t.Fatal("follow.created only for a new follow")
	}
}

func TestFollowBlockRace(t *testing.T) {
	e := newEnv(t)
	e.expectUser(usr(other))
	e.expectBlocked(false)
	e.mock.ExpectBegin()
	e.expectInsertFollow(false) // a block appeared after the check
	e.expectCount(other, 0)
	e.mock.ExpectCommit()
	e.expectFollows(me, other, false)
	status(t, e.do(http.MethodPost, "/users/9/follow"), http.StatusNotFound)
	if len(e.created) != 0 {
		t.Fatal("no event")
	}
}

func TestFollowSelfIs422(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/users/7/follow", "/users/7/block"} {
		for _, m := range []string{http.MethodPost, http.MethodDelete} {
			b := status(t, e.do(m, p), http.StatusUnprocessableEntity)
			if b.Error.Fields["userId"] == "" {
				t.Fatalf("%s %s: %+v", m, p, b.Error)
			}
		}
	}
}

func TestTargetNotFoundCases(t *testing.T) {
	cases := map[string]func(e *env){
		"unknown": func(e *env) { e.expectNoUser(other) },
		"deleted": func(e *env) {
			u := usr(other)
			u.Status = "deleted"
			e.expectUser(u)
		},
		"suspended": func(e *env) {
			u := usr(other)
			u.Status = "suspended"
			e.expectUser(u)
		},
		"not onboarded": func(e *env) {
			u := usr(other)
			u.OnboardingStep = "profile"
			e.expectUser(u)
		},
		"blocked either way": func(e *env) {
			e.expectUser(usr(other))
			e.expectBlocked(true)
		},
	}
	for name, setup := range cases {
		for _, rt := range [][2]string{{http.MethodPost, "/users/9/follow"}, {http.MethodGet, "/users/9/followers"}, {http.MethodGet, "/users/9/following"}} {
			t.Run(name+" "+rt[0]+" "+rt[1], func(t *testing.T) {
				e := newEnv(t)
				setup(e)
				if b := status(t, e.do(rt[0], rt[1]), http.StatusNotFound); b.Error.Code != "NOT_FOUND" {
					t.Fatalf("%+v", b.Error)
				}
			})
		}
	}
	e := newEnv(t)
	status(t, e.do(http.MethodPost, "/users/abc/follow"), http.StatusNotFound)
}

func TestUnfollow(t *testing.T) {
	e := newEnv(t)
	u := usr(other)
	u.Status = "suspended" // unfollowing a suspended user is allowed
	e.expectUser(u)
	e.expectBlocked(false)
	e.mock.ExpectBegin()
	e.expectDeleteFollow(me, other, true)
	e.expectAdjust("followers_count", other, -1)
	e.expectAdjust("following_count", me, -1)
	e.expectCount(other, 3)
	e.mock.ExpectCommit()
	if s := state(t, status(t, e.do(http.MethodDelete, "/users/9/follow"), http.StatusOK)); s != (State{Following: false, FollowersCount: 3}) {
		t.Fatalf("state = %+v", s)
	}
	// Idempotent: nothing to remove, counters untouched.
	e.expectBlocked(false)
	e.mock.ExpectBegin()
	e.expectDeleteFollow(me, other, false)
	e.expectCount(other, 3)
	e.mock.ExpectCommit()
	status(t, e.do(http.MethodDelete, "/users/9/follow"), http.StatusOK)

	e2 := newEnv(t)
	e2.expectUser(usr(other))
	e2.expectBlocked(true)
	status(t, e2.do(http.MethodDelete, "/users/9/follow"), http.StatusNotFound)
}

func TestFollowDBErrorRollsBack(t *testing.T) {
	e := newEnv(t)
	e.expectUser(usr(other))
	e.expectBlocked(false)
	e.mock.ExpectBegin()
	e.expectInsertFollow(true)
	e.mock.ExpectExec(q(`UPDATE profiles SET followers_count`)).WillReturnError(errors.New("db down"))
	e.mock.ExpectRollback()
	status(t, e.do(http.MethodPost, "/users/9/follow"), http.StatusInternalServerError)
	if len(e.created) != 0 {
		t.Fatal("no event before commit")
	}
}

// ---- block ----

func TestBlockRemovesFollowsBothWays(t *testing.T) {
	e := newEnv(t)
	u := usr(other)
	u.Status = "suspended"
	e.expectUser(u) // no block check: blocking back is allowed
	e.mock.ExpectBegin()
	e.mock.ExpectExec(q(`INSERT IGNORE INTO blocks (blocker_id, blocked_id) VALUES (?, ?)`)).WithArgs(me, other).WillReturnResult(sqlmock.NewResult(0, 1))
	e.expectDeleteFollow(me, other, true)
	e.expectAdjust("followers_count", other, -1)
	e.expectAdjust("following_count", me, -1)
	e.expectDeleteFollow(other, me, true)
	e.expectAdjust("followers_count", me, -1)
	e.expectAdjust("following_count", other, -1)
	e.mock.ExpectCommit()
	status(t, e.do(http.MethodPost, "/users/9/block"), http.StatusNoContent)

	// Again: the block exists, no follows left, no counter changes.
	e.mock.ExpectBegin()
	e.mock.ExpectExec(q(`INSERT IGNORE INTO blocks`)).WithArgs(me, other).WillReturnResult(sqlmock.NewResult(0, 0))
	e.expectDeleteFollow(me, other, false)
	e.expectDeleteFollow(other, me, false)
	e.mock.ExpectCommit()
	status(t, e.do(http.MethodPost, "/users/9/block"), http.StatusNoContent)

	e2 := newEnv(t)
	e2.expectNoUser(other)
	status(t, e2.do(http.MethodPost, "/users/9/block"), http.StatusNotFound)
}

func TestUnblock(t *testing.T) {
	e := newEnv(t)
	e.mock.ExpectExec(q(`DELETE FROM blocks WHERE blocker_id = ? AND blocked_id = ?`)).WithArgs(me, other).WillReturnResult(sqlmock.NewResult(0, 1))
	status(t, e.do(http.MethodDelete, "/users/9/block"), http.StatusNoContent)
	e.mock.ExpectExec(q(`DELETE FROM blocks WHERE blocker_id = ? AND blocked_id = ?`)).WithArgs(me, other).WillReturnResult(sqlmock.NewResult(0, 0))
	status(t, e.do(http.MethodDelete, "/users/9/block"), http.StatusNoContent)
}

// ---- lists ----

var (
	t1 = time.Date(2026, 6, 10, 8, 0, 0, 123000000, time.UTC)
	t2 = time.Date(2026, 6, 9, 8, 0, 0, 0, time.UTC)
	t3 = time.Date(2026, 6, 8, 8, 0, 0, 0, time.UTC)
)

func listRows(rows ...[2]any) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"id", "created_at"})
	for _, x := range rows {
		r.AddRow(x[0], x[1])
	}
	return r
}

func (e *env) expectSummaries(ids ...int64) {
	args := make([]driver.Value, len(ids))
	rows := sqlmock.NewRows([]string{"id", "display_name", "primary_role", "verification_status", "photo_upload_id", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"})
	// Returned in a different order than asked: the list keeps its own order.
	for i := range ids {
		args[i] = ids[i]
		id := ids[len(ids)-1-i]
		rows.AddRow(id, "U", "player", "verified", nil, "Pune", "Maharashtra", nil, nil, nil, nil, nil, nil)
	}
	e.mock.ExpectQuery(q(`SELECT u.id, COALESCE(u.display_name,'')`)).WithArgs(args...).WillReturnRows(rows)
}

func (e *env) expectMinors(ids []int64, minors ...int64) {
	args := []driver.Value{}
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, me)
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range minors {
		rows.AddRow(id)
	}
	e.mock.ExpectQuery(q(`SELECT u.id FROM users u WHERE u.is_minor = 1`)).WithArgs(args...).WillReturnRows(rows)
}

type card struct {
	ID   int64   `json:"id"`
	City *string `json:"city"`
}

func cards(t *testing.T, b body) []card {
	t.Helper()
	var out []card
	if err := json.Unmarshal(b.Data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

const followersBase = `SELECT f.follower_id, f.created_at FROM follows f JOIN users u ON u.id = f.follower_id WHERE f.followee_id = ? AND u.status = 'active' AND NOT EXISTS`

func TestOwnFollowersPagination(t *testing.T) {
	e := newEnv(t)
	e.mock.ExpectQuery(q(followersBase)+`.*`+q(`ORDER BY f.created_at DESC, f.follower_id DESC LIMIT ?`)).WithArgs(me, me, me, 3).
		WillReturnRows(listRows([2]any{int64(11), t1}, [2]any{int64(12), t2}, [2]any{int64(13), t3}))
	e.expectSummaries(11, 12)
	e.expectMinors([]int64{11, 12}, 12)
	b := status(t, e.do(http.MethodGet, "/users/7/followers?limit=2"), http.StatusOK)
	list := cards(t, b)
	if len(list) != 2 || list[0].ID != 11 || list[1].ID != 12 || list[0].City == nil || list[1].City != nil {
		t.Fatalf("list = %+v", list)
	}
	next, _ := b.Meta["nextCursor"].(string)
	if next != httpx.EncodeCursor(httpx.Cursor{ID: 12, Time: "2026-06-09T08:00:00Z"}) {
		t.Fatalf("next = %q", next)
	}

	e.mock.ExpectQuery(q(`AND (f.created_at < ? OR (f.created_at = ? AND f.follower_id < ?)) ORDER BY`)).WithArgs(me, me, me, t2, t2, int64(12), 3).
		WillReturnRows(listRows([2]any{int64(13), t3}))
	e.expectSummaries(13)
	e.expectMinors([]int64{13})
	b = status(t, e.do(http.MethodGet, "/users/7/followers?limit=2&cursor="+next), http.StatusOK)
	if l := cards(t, b); len(l) != 1 || l[0].ID != 13 || b.Meta["nextCursor"] != nil {
		t.Fatalf("page 2 = %+v %v", l, b.Meta)
	}
}

func TestListBadCursor(t *testing.T) {
	e := newEnv(t)
	for _, c := range []httpx.Cursor{{ID: 3, Time: "yesterday"}, {Time: "2026-06-09T08:00:00Z"}} {
		status(t, e.do(http.MethodGet, "/users/7/followers?cursor="+httpx.EncodeCursor(c)), http.StatusUnprocessableEntity)
		status(t, e.do(http.MethodGet, "/me/blocks?cursor="+httpx.EncodeCursor(c)), http.StatusUnprocessableEntity)
	}
	status(t, e.do(http.MethodGet, "/users/7/following?cursor=not*base64"), http.StatusUnprocessableEntity)
}

func (e *env) expectVisibility(v string) {
	rows := sqlmock.NewRows([]string{"visibility"})
	if v != "" {
		rows.AddRow(v)
	}
	e.mock.ExpectQuery(q(`SELECT visibility FROM profiles WHERE user_id = ?`)).WithArgs(other).WillReturnRows(rows)
}

func TestOtherUsersListsFollowProfileVisibility(t *testing.T) {
	for _, vis := range []string{"private", "", "followers"} {
		t.Run("hidden "+vis, func(t *testing.T) {
			e := newEnv(t)
			e.expectUser(usr(other))
			e.expectBlocked(false)
			e.expectVisibility(vis)
			if vis == "followers" {
				e.expectFollows(me, other, false)
			}
			b := status(t, e.do(http.MethodGet, "/users/9/following"), http.StatusOK)
			if string(b.Data) != "[]" || b.Meta["nextCursor"] != nil {
				t.Fatalf("%s", b.Data)
			}
		})
	}
	e := newEnv(t)
	e.expectUser(usr(other))
	e.expectBlocked(false)
	e.expectVisibility("followers")
	e.expectFollows(me, other, true)
	e.mock.ExpectQuery(q(`SELECT f.followee_id, f.created_at FROM follows f JOIN users u ON u.id = f.followee_id WHERE f.follower_id = ?`)+`.*`+q(`LIMIT ?`)).
		WithArgs(other, me, me, 21).WillReturnRows(listRows([2]any{int64(11), t1}, [2]any{me, t2}))
	e.expectSummaries(11, me)
	e.expectMinors([]int64{11}) // the viewer's own card is never checked
	if l := cards(t, status(t, e.do(http.MethodGet, "/users/9/following"), http.StatusOK)); len(l) != 2 || l[0].ID != 11 || l[1].ID != me {
		t.Fatalf("list = %+v", l)
	}

	e2 := newEnv(t)
	e2.expectUser(usr(other))
	e2.expectBlocked(false)
	e2.expectVisibility("public")
	e2.mock.ExpectQuery(q(followersBase)).WithArgs(other, me, me, 21).WillReturnRows(listRows())
	status(t, e2.do(http.MethodGet, "/users/9/followers"), http.StatusOK)
}

func TestBlocksList(t *testing.T) {
	e := newEnv(t)
	e.mock.ExpectQuery(q(`SELECT b.blocked_id, b.created_at FROM blocks b JOIN users u ON u.id = b.blocked_id WHERE b.blocker_id = ? AND u.status <> 'deleted' ORDER BY b.created_at DESC, b.blocked_id DESC LIMIT ?`)).
		WithArgs(me, 21).WillReturnRows(listRows([2]any{other, t1}, [2]any{int64(11), t2}))
	// A card that disappeared in between (deleted) is skipped.
	e.mock.ExpectQuery(q(`SELECT u.id, COALESCE(u.display_name,'')`)).WithArgs(other, int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "display_name", "primary_role", "verification_status", "photo_upload_id", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"}).
			AddRow(other, "U", "player", "verified", nil, nil, nil, nil, nil, nil, nil, nil, nil))
	if l := cards(t, status(t, e.do(http.MethodGet, "/me/blocks"), http.StatusOK)); len(l) != 1 || l[0].ID != other {
		t.Fatalf("list = %+v", l)
	}
	e.mock.ExpectQuery(q(`AND (b.created_at < ? OR (b.created_at = ? AND b.blocked_id < ?))`)).WithArgs(me, t1, t1, other, 21).WillReturnRows(listRows())
	cur := httpx.EncodeCursor(httpx.Cursor{ID: other, Time: t1.Format(time.RFC3339Nano)})
	status(t, e.do(http.MethodGet, "/me/blocks?cursor="+cur), http.StatusOK)
}

// ---- user.deleted ----

func expectCleanup(e *env, followees, followers []int64) {
	e.mock.ExpectBegin()
	r1 := sqlmock.NewRows([]string{"id"})
	for _, id := range followees {
		r1.AddRow(id)
	}
	r2 := sqlmock.NewRows([]string{"id"})
	for _, id := range followers {
		r2.AddRow(id)
	}
	e.mock.ExpectQuery(q(`SELECT followee_id FROM follows WHERE follower_id = ? FOR UPDATE`)).WithArgs(me).WillReturnRows(r1)
	e.mock.ExpectQuery(q(`SELECT follower_id FROM follows WHERE followee_id = ? FOR UPDATE`)).WithArgs(me).WillReturnRows(r2)
	for _, id := range followees {
		e.expectAdjust("followers_count", id, -1)
	}
	for _, id := range followers {
		e.expectAdjust("following_count", id, -1)
	}
	e.mock.ExpectExec(q(`DELETE FROM follows WHERE follower_id = ? OR followee_id = ?`)).WithArgs(me, me).WillReturnResult(sqlmock.NewResult(0, 3))
	e.mock.ExpectExec(q(`DELETE FROM blocks WHERE blocker_id = ? OR blocked_id = ?`)).WithArgs(me, me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
}

func TestUserDeletedIsIdempotent(t *testing.T) {
	e := newEnv(t)
	h := e.m.EventHandlers()[events.UserDeleted]
	ev := events.New(events.UserDeleted, me, events.UserPayload{UserID: me})
	expectCleanup(e, []int64{other, 11}, []int64{12})
	expectCleanup(e, nil, nil) // second delivery: nothing left, no counter changes
	for i := 0; i < 2; i++ {
		if err := h(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := h(context.Background(), events.New(events.UserDeleted, 0, events.UserPayload{})); err != nil {
		t.Fatal("malformed payload is dropped")
	}
	e.mock.ExpectBegin()
	e.mock.ExpectQuery(q(`SELECT followee_id FROM follows`)).WillReturnError(errors.New("db down"))
	e.mock.ExpectRollback()
	if err := h(context.Background(), ev); err == nil {
		t.Fatal("db errors are returned for retry")
	}
}

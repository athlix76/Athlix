package moderation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/testutil"
)

const (
	reporter = int64(42)
	adminID  = int64(1)
)

var (
	created = time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)
	report  = map[string]any{"targetType": "post", "targetId": 70, "reason": "spam"}
)

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
		t.Fatalf("decode %s: %v", body, err)
	}
	return e
}

func userSetup(t *testing.T, cfg map[string]any) (*app.Deps, sqlmock.Sqlmock, *gin.Engine) {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	r, routes := testutil.UserEngine(testutil.VerifiedUser(reporter))
	New(d).Routes(routes)
	return d, mock, r
}

func adminSetup(t *testing.T, cfg map[string]any, perms ...string) (*app.Deps, sqlmock.Sqlmock, *gin.Engine) {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	r, routes := testutil.AdminEngine(adminID, perms)
	New(d).Routes(routes)
	return d, mock, r
}

func capture(d *app.Deps, typ string) *[]events.Event {
	got := &[]events.Event{}
	d.Dispatcher.On(typ, func(_ context.Context, e events.Event) error { *got = append(*got, e); return nil })
	return got
}

func count(n int) *sqlmock.Rows { return sqlmock.NewRows([]string{"n"}).AddRow(n) }

// expectNewPostReport covers visibility, no existing report, insert and open count.
func expectNewPostReport(mock sqlmock.Sqlmock, open int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM posts p WHERE p.id = \?`).
		WithArgs(70, reporter, reporter, reporter, reporter).WillReturnRows(count(1))
	mock.ExpectQuery(`SELECT id FROM reports WHERE reporter_id = \?`).
		WithArgs(reporter, "post", 70).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO reports`).WithArgs(reporter, "post", 70, "spam", nil).
		WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM reports WHERE target_type = \? AND target_id = \? AND status = 'open'`).
		WithArgs("post", 70).WillReturnRows(count(open))
}

func TestCreateReport_Created(t *testing.T) {
	d, mock, r := userSetup(t, nil)
	got := capture(d, events.ReportCreated)
	expectNewPostReport(mock, 1)
	mock.ExpectCommit()

	w := testutil.Request(r, http.MethodPost, "/v1/reports", report, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	if string(decode(t, w.Body.Bytes()).Data) != `{"id":9}` {
		t.Fatalf("body %s", w.Body)
	}
	if len(*got) != 1 {
		t.Fatalf("report.created published %d times", len(*got))
	}
	var p events.ReportPayload
	_ = (*got)[0].Decode(&p)
	if p.ReportID != 9 || p.TargetType != "post" || p.TargetID != 70 {
		t.Fatalf("payload %+v", p)
	}
}

func TestCreateReport_DuplicateReturnsExisting(t *testing.T) {
	d, mock, r := userSetup(t, nil)
	got := capture(d, events.ReportCreated)
	mock.ExpectQuery(`FROM posts p`).WillReturnRows(count(1))
	mock.ExpectQuery(`SELECT id FROM reports WHERE reporter_id = \?`).
		WithArgs(reporter, "post", 70).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5))

	w := testutil.Request(r, http.MethodPost, "/v1/reports", report, nil)
	if w.Code != http.StatusOK || string(decode(t, w.Body.Bytes()).Data) != `{"id":5}` {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	if len(*got) != 0 {
		t.Fatal("duplicate must not publish")
	}
}

func TestCreateReport_DuplicateOnInsertRace(t *testing.T) {
	_, mock, r := userSetup(t, nil)
	mock.ExpectQuery(`FROM posts p`).WillReturnRows(count(1))
	mock.ExpectQuery(`SELECT id FROM reports`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO reports`).WillReturnError(errors.New("Error 1062: Duplicate entry '42-post-70'"))
	mock.ExpectRollback()
	mock.ExpectQuery(`SELECT id FROM reports`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(6))

	w := testutil.Request(r, http.MethodPost, "/v1/reports", report, nil)
	if w.Code != http.StatusOK || string(decode(t, w.Body.Bytes()).Data) != `{"id":6}` {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestCreateReport_AutoHideAtThreshold(t *testing.T) {
	_, mock, r := userSetup(t, map[string]any{"moderation.auto_hide_report_threshold": 3})
	expectNewPostReport(mock, 3)
	mock.ExpectExec(`UPDATE posts SET status = 'hidden' WHERE id = \? AND status = 'visible'`).
		WithArgs(70).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO moderation_actions`).
		WithArgs(nil, "post", 70, "auto_hide", sqlmock.AnyArg(), 9).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if w := testutil.Request(r, http.MethodPost, "/v1/reports", report, nil); w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestCreateReport_AutoHideIdempotent(t *testing.T) {
	_, mock, r := userSetup(t, map[string]any{"moderation.auto_hide_report_threshold": 3})
	expectNewPostReport(mock, 4)
	// already hidden: 0 rows affected, so no second auto_hide action
	mock.ExpectExec(`UPDATE posts SET status = 'hidden'`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if w := testutil.Request(r, http.MethodPost, "/v1/reports", report, nil); w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestCreateReport_BelowThresholdNoHide(t *testing.T) {
	// 3 open reports hide with threshold 3 (above) but not with the default 5.
	_, mock, r := userSetup(t, nil)
	expectNewPostReport(mock, 3)
	mock.ExpectCommit()
	if w := testutil.Request(r, http.MethodPost, "/v1/reports", report, nil); w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestCreateReport_UserTargetsAreOnlyQueued(t *testing.T) {
	_, mock, r := userSetup(t, map[string]any{"moderation.auto_hide_report_threshold": 1})
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE id = \? AND status = 'active'`).WithArgs(50).WillReturnRows(count(1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM blocks`).WithArgs(reporter, 50, 50, reporter).WillReturnRows(count(0))
	mock.ExpectQuery(`SELECT id FROM reports`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO reports`).WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectCommit()

	body := map[string]any{"targetType": "user", "targetId": 50, "reason": "fake_profile", "details": "  pretends to be a coach "}
	if w := testutil.Request(r, http.MethodPost, "/v1/reports", body, nil); w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestCreateReport_NotVisible404(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		mock func(sqlmock.Sqlmock)
	}{
		{"hidden post", report, func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`FROM posts p`).WillReturnRows(count(0))
		}},
		{"blocked user", map[string]any{"targetType": "user", "targetId": 50, "reason": "spam"}, func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`FROM users WHERE id = \?`).WillReturnRows(count(1))
			m.ExpectQuery(`FROM blocks`).WillReturnRows(count(1))
		}},
		{"draft event of someone else", map[string]any{"targetType": "event", "targetId": 9, "reason": "scam"}, func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`FROM events WHERE id = \?`).WithArgs(9, reporter).WillReturnRows(count(0))
		}},
		{"message outside my conversations", map[string]any{"targetType": "message", "targetId": 3, "reason": "harassment"}, func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`FROM messages m`).WithArgs(3, reporter).WillReturnRows(count(0))
		}},
		{"private conversation", map[string]any{"targetType": "conversation", "targetId": 4, "reason": "spam"}, func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`FROM conversations c`).WithArgs(reporter, 4).WillReturnRows(count(0))
		}},
		{"comment on hidden post", map[string]any{"targetType": "comment", "targetId": 5, "reason": "spam"}, func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`FROM post_comments c JOIN posts p`).WillReturnRows(count(0))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, mock, r := userSetup(t, nil)
			tc.mock(mock)
			w := testutil.Request(r, http.MethodPost, "/v1/reports", tc.body, nil)
			if w.Code != http.StatusNotFound || decode(t, w.Body.Bytes()).Error.Code != "NOT_FOUND" {
				t.Fatalf("status %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestCreateReport_Validation(t *testing.T) {
	long := make([]byte, 1001)
	for i := range long {
		long[i] = 'a'
	}
	cases := []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"targetType": "page", "targetId": 1, "reason": "spam"}, "targetType"},
		{map[string]any{"targetType": "post", "reason": "spam"}, "targetId"},
		{map[string]any{"targetType": "post", "targetId": 1, "reason": "boring"}, "reason"},
		{map[string]any{"targetType": "post", "targetId": 1, "reason": "spam", "details": string(long)}, "details"},
		{map[string]any{"targetType": "user", "targetId": reporter, "reason": "spam"}, "targetId"},
	}
	for _, tc := range cases {
		_, _, r := userSetup(t, nil)
		w := testutil.Request(r, http.MethodPost, "/v1/reports", tc.body, nil)
		e := decode(t, w.Body.Bytes())
		if w.Code != http.StatusUnprocessableEntity || e.Error.Fields[tc.field] == "" {
			t.Fatalf("%v: status %d %s", tc.body, w.Code, w.Body)
		}
	}
}

func TestCreateReport_RateLimited(t *testing.T) {
	d, _, r := userSetup(t, nil)
	if err := d.Redis.Set(context.Background(), "rl:report:42", 20, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	w := testutil.Request(r, http.MethodPost, "/v1/reports", report, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

// ---- admin ----

var reportHdr = []string{"id", "reporter_id", "target_type", "target_id", "reason", "details", "status", "resolution_note", "created_at", "resolved_at", "open"}

func reportRows(id int64, targetType string, targetID int64, reason, status string) *sqlmock.Rows {
	var resolved any
	if status != StatusOpen {
		resolved = created
	}
	return sqlmock.NewRows(reportHdr).AddRow(id, reporter, targetType, targetID, reason, nil, status, nil, created, resolved, 2)
}

func summaryRows(ids ...int64) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "name", "role", "verification", "photo", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"})
	for _, id := range ids {
		rows.AddRow(id, "Asha K", "player", "verified", nil, "Pune", "Maharashtra", nil, nil, nil, nil, nil, nil)
	}
	return rows
}

func expectAudit(mock sqlmock.Sqlmock, action string) {
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(adminID, action, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectReload(mock sqlmock.Sqlmock, id int64, targetType string, targetID int64, status string, previewRe string, preview *sqlmock.Rows) {
	mock.ExpectQuery(`FROM reports r WHERE r.id = \?`).WithArgs(id).WillReturnRows(reportRows(id, targetType, targetID, "spam", status))
	mock.ExpectQuery(`FROM users u\s+LEFT JOIN profiles p ON p.user_id = u.id\s+LEFT JOIN sports`).WithArgs(reporter).WillReturnRows(summaryRows(reporter))
	mock.ExpectQuery(previewRe).WithArgs(targetID).WillReturnRows(preview)
}

func TestAdmin_PermissionRequired(t *testing.T) {
	cases := []struct{ method, path string }{
		{http.MethodGet, "/v1/admin/reports"},
		{http.MethodPost, "/v1/admin/reports/1/resolve"},
		{http.MethodGet, "/v1/admin/moderation/actions"},
		{http.MethodPost, "/v1/admin/moderation/actions"},
	}
	for _, tc := range cases {
		_, _, r := adminSetup(t, nil, "dashboard.read")
		if w := testutil.Request(r, tc.method, tc.path, map[string]any{}, nil); w.Code != http.StatusForbidden {
			t.Fatalf("%s %s: status %d", tc.method, tc.path, w.Code)
		}
	}
	// reports.read alone cannot resolve
	_, _, r := adminSetup(t, nil, "reports.read")
	if w := testutil.Request(r, http.MethodPost, "/v1/admin/reports/1/resolve", map[string]any{"action": "dismiss"}, nil); w.Code != http.StatusForbidden {
		t.Fatalf("status %d", w.Code)
	}
}

func TestAdmin_ResolveHideAlreadyAutoHidden(t *testing.T) {
	_, mock, r := adminSetup(t, nil, "reports.resolve")
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM reports r WHERE r.id = \? FOR UPDATE`).WithArgs(1).WillReturnRows(reportRows(1, "post", 70, "spam", StatusOpen))
	mock.ExpectQuery(`SELECT status, author_id, 0 FROM posts WHERE id = \? FOR UPDATE`).WithArgs(70).
		WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("hidden", 50, 0))
	mock.ExpectExec(`INSERT INTO moderation_actions`).WithArgs(adminID, "post", 70, "hide", "confirmed spam", 1).WillReturnResult(sqlmock.NewResult(3, 1))
	mock.ExpectExec(`UPDATE reports SET status = 'actioned'`).WithArgs(adminID, "confirmed spam", sqlmock.AnyArg(), "post", 70).WillReturnResult(sqlmock.NewResult(0, 2))
	expectAudit(mock, "report.resolve")
	mock.ExpectCommit()
	expectReload(mock, 1, "post", 70, StatusActioned, `SELECT id, NULL, body, author_id, NULL FROM posts WHERE id IN`,
		sqlmock.NewRows([]string{"id", "t", "x", "o", "c"}).AddRow(70, nil, "Buy followers now", 50, nil))

	w := testutil.Request(r, http.MethodPost, "/v1/admin/reports/1/resolve", map[string]any{"action": "hide", "note": "confirmed spam"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var got AdminReport
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &got)
	if got.Status != StatusActioned || got.Reporter == nil || got.Reporter.ID != reporter ||
		got.TargetPreview.Text == nil || *got.TargetPreview.Text != "Buy followers now" || *got.TargetPreview.OwnerID != 50 || got.ResolvedAt == nil {
		t.Fatalf("report %+v", got)
	}
}

func TestAdmin_ResolveRemoveEventPublishesCancelled(t *testing.T) {
	d, mock, r := adminSetup(t, nil, "reports.resolve")
	got := capture(d, events.EventCancelled)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM reports r WHERE r.id = \? FOR UPDATE`).WillReturnRows(reportRows(2, "event", 9, "scam", StatusOpen))
	mock.ExpectQuery(`SELECT status, organiser_id, sport_id FROM events WHERE id = \? FOR UPDATE`).WithArgs(9).
		WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("published", 77, 3))
	mock.ExpectExec(`UPDATE events SET status = \? WHERE id = \?`).WithArgs("removed", 9).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO moderation_actions`).WithArgs(adminID, "event", 9, "remove", nil, 2).WillReturnResult(sqlmock.NewResult(4, 1))
	mock.ExpectExec(`UPDATE reports SET status = 'actioned'`).WillReturnResult(sqlmock.NewResult(0, 1))
	expectAudit(mock, "report.resolve")
	mock.ExpectCommit()
	expectReload(mock, 2, "event", 9, StatusActioned, `SELECT id, title, description, organiser_id, NULL FROM events WHERE id IN`,
		sqlmock.NewRows([]string{"id", "t", "x", "o", "c"}).AddRow(9, "Cup", nil, 77, nil))

	w := testutil.Request(r, http.MethodPost, "/v1/admin/reports/2/resolve", map[string]any{"action": "remove"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	if len(*got) != 1 {
		t.Fatalf("event.cancelled published %d times", len(*got))
	}
	var p events.EventPayload
	_ = (*got)[0].Decode(&p)
	if p.EventID != 9 || p.OrganiserID != 77 || p.SportID != 3 || p.Reason == "" {
		t.Fatalf("payload %+v", p)
	}
}

func TestAdmin_ResolveEventInWrongState409(t *testing.T) {
	d, mock, r := adminSetup(t, nil, "reports.resolve")
	got := capture(d, events.EventCancelled)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM reports r WHERE r.id = \? FOR UPDATE`).WillReturnRows(reportRows(2, "event", 9, "scam", StatusOpen))
	mock.ExpectQuery(`FROM events WHERE id = \? FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("completed", 77, 3))
	mock.ExpectRollback()

	w := testutil.Request(r, http.MethodPost, "/v1/admin/reports/2/resolve", map[string]any{"action": "remove"}, nil)
	if w.Code != http.StatusConflict || decode(t, w.Body.Bytes()).Error.Code != "INVALID_STATE" {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	if len(*got) != 0 {
		t.Fatal("no event on failure")
	}
}

func TestAdmin_ResolveSuspendsOwnerOfPost(t *testing.T) {
	_, mock, r := adminSetup(t, nil, "reports.resolve")
	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WillReturnRows(reportRows(3, "post", 70, "harassment", StatusOpen))
	mock.ExpectQuery(`FROM posts WHERE id = \? FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("visible", 50, 0))
	mock.ExpectQuery(`SELECT status, id, 0 FROM users WHERE id = \? FOR UPDATE`).WithArgs(50).
		WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("active", 50, 0))
	mock.ExpectExec(`UPDATE users SET status = \? WHERE id = \?`).WithArgs("suspended", 50).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO moderation_actions`).WithArgs(adminID, "user", 50, "suspend_user", nil, 3).WillReturnResult(sqlmock.NewResult(5, 1))
	mock.ExpectExec(`UPDATE reports SET status = 'actioned'`).WithArgs(adminID, nil, sqlmock.AnyArg(), "post", 70).WillReturnResult(sqlmock.NewResult(0, 1))
	expectAudit(mock, "report.resolve")
	mock.ExpectCommit()
	expectReload(mock, 3, "post", 70, StatusActioned, `FROM posts WHERE id IN`, sqlmock.NewRows([]string{"id", "t", "x", "o", "c"}))

	if w := testutil.Request(r, http.MethodPost, "/v1/admin/reports/3/resolve", map[string]any{"action": "suspend_user"}, nil); w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestAdmin_ResolveDeletedUserNotTouched(t *testing.T) {
	_, mock, r := adminSetup(t, nil, "reports.resolve")
	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WillReturnRows(reportRows(3, "user", 50, "spam", StatusOpen))
	mock.ExpectQuery(`FROM users WHERE id = \? FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("deleted", 50, 0))
	mock.ExpectRollback()
	if w := testutil.Request(r, http.MethodPost, "/v1/admin/reports/3/resolve", map[string]any{"action": "suspend_user"}, nil); w.Code != http.StatusConflict {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestAdmin_DismissLiftsAutoHide(t *testing.T) {
	_, mock, r := adminSetup(t, map[string]any{"moderation.auto_hide_report_threshold": 2}, "reports.resolve")
	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WillReturnRows(reportRows(4, "comment", 5, "spam", StatusOpen))
	mock.ExpectExec(`UPDATE reports SET status = \?, resolved_by = \?`).WithArgs("dismissed", adminID, nil, sqlmock.AnyArg(), 4).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM reports WHERE target_type = \?`).WithArgs("comment", 5).WillReturnRows(count(1))
	mock.ExpectQuery(`SELECT action FROM moderation_actions`).WithArgs("comment", 5).WillReturnRows(sqlmock.NewRows([]string{"action"}).AddRow("auto_hide"))
	mock.ExpectQuery(`FROM post_comments WHERE id = \? FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("hidden", 50, 0))
	mock.ExpectExec(`UPDATE post_comments SET status = \? WHERE id = \?`).WithArgs("visible", 5).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO moderation_actions`).WithArgs(adminID, "comment", 5, "unhide", sqlmock.AnyArg(), 4).WillReturnResult(sqlmock.NewResult(6, 1))
	expectAudit(mock, "report.resolve")
	mock.ExpectCommit()
	expectReload(mock, 4, "comment", 5, StatusDismissed, `FROM post_comments WHERE id IN`, sqlmock.NewRows([]string{"id", "t", "x", "o", "c"}))

	if w := testutil.Request(r, http.MethodPost, "/v1/admin/reports/4/resolve", map[string]any{"action": "dismiss"}, nil); w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestAdmin_DismissKeepsHiddenWhileAboveThreshold(t *testing.T) {
	_, mock, r := adminSetup(t, map[string]any{"moderation.auto_hide_report_threshold": 2}, "reports.resolve")
	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WillReturnRows(reportRows(4, "comment", 5, "spam", StatusOpen))
	mock.ExpectExec(`UPDATE reports SET status = \?`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM reports`).WillReturnRows(count(2))
	expectAudit(mock, "report.resolve")
	mock.ExpectCommit()
	expectReload(mock, 4, "comment", 5, StatusDismissed, `FROM post_comments WHERE id IN`, sqlmock.NewRows([]string{"id", "t", "x", "o", "c"}))

	if w := testutil.Request(r, http.MethodPost, "/v1/admin/reports/4/resolve", map[string]any{"action": "dismiss"}, nil); w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestAdmin_ResolveErrors(t *testing.T) {
	t.Run("validation", func(t *testing.T) {
		_, _, r := adminSetup(t, nil, "reports.resolve")
		w := testutil.Request(r, http.MethodPost, "/v1/admin/reports/1/resolve", map[string]any{"action": "unhide"}, nil)
		if w.Code != http.StatusUnprocessableEntity || decode(t, w.Body.Bytes()).Error.Fields["action"] == "" {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
	})
	t.Run("not found", func(t *testing.T) {
		_, mock, r := adminSetup(t, nil, "reports.resolve")
		mock.ExpectBegin()
		mock.ExpectQuery(`FOR UPDATE`).WillReturnRows(sqlmock.NewRows(reportHdr))
		mock.ExpectRollback()
		if w := testutil.Request(r, http.MethodPost, "/v1/admin/reports/99/resolve", map[string]any{"action": "dismiss"}, nil); w.Code != http.StatusNotFound {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
	})
	t.Run("already resolved", func(t *testing.T) {
		_, mock, r := adminSetup(t, nil, "reports.resolve")
		mock.ExpectBegin()
		mock.ExpectQuery(`FOR UPDATE`).WillReturnRows(reportRows(1, "post", 70, "spam", StatusDismissed))
		mock.ExpectRollback()
		w := testutil.Request(r, http.MethodPost, "/v1/admin/reports/1/resolve", map[string]any{"action": "dismiss"}, nil)
		if w.Code != http.StatusConflict || decode(t, w.Body.Bytes()).Error.Code != "INVALID_STATE" {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
	})
}

func TestAdmin_ListReportsPagesAndHidesMessageText(t *testing.T) {
	_, mock, r := adminSetup(t, nil, "reports.read")
	rows := sqlmock.NewRows(reportHdr).
		AddRow(8, reporter, "message", 3, "underage_safety", nil, "open", nil, created, nil, 1).
		AddRow(7, reporter, "message", 4, "underage_safety", nil, "open", nil, created, nil, 1)
	mock.ExpectQuery(`FROM reports r WHERE 1 = 1 AND r.status = \? AND r.target_type = \? ORDER BY \(r.reason = 'underage_safety'\) DESC, r.id DESC LIMIT \?`).
		WithArgs("open", "message", 2).WillReturnRows(rows)
	mock.ExpectQuery(`FROM users u`).WillReturnRows(summaryRows(reporter))
	mock.ExpectQuery(`SELECT id, NULL, body, sender_id, conversation_id FROM messages WHERE id IN`).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"id", "t", "x", "o", "c"}).AddRow(3, nil, "private words", 60, 4))

	w := testutil.Request(r, http.MethodGet, "/v1/admin/reports?status=open&targetType=message&limit=1", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	e := decode(t, w.Body.Bytes())
	var items []AdminReport
	_ = json.Unmarshal(e.Data, &items)
	if len(items) != 1 || items[0].TargetPreview.Text != nil || items[0].TargetPreview.OwnerID == nil || e.Meta["nextCursor"] == nil {
		t.Fatalf("items %s meta %v", e.Data, e.Meta)
	}

	// next page from an underage_safety bucket keeps the rest of that bucket, then the others
	mock.ExpectQuery(`AND \(\(r.reason = 'underage_safety' AND r.id < \?\) OR r.reason <> 'underage_safety'\)`).
		WithArgs(8, 21).WillReturnRows(sqlmock.NewRows(reportHdr))
	w = testutil.Request(r, http.MethodGet, "/v1/admin/reports?cursor="+e.Meta["nextCursor"].(string), nil, nil)
	if w.Code != http.StatusOK || string(decode(t, w.Body.Bytes()).Data) != "[]" {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestAdmin_ListReportsMessageTextIsAudited(t *testing.T) {
	_, mock, r := adminSetup(t, nil, "reports.read", "moderation.read_messages")
	mock.ExpectQuery(`FROM reports r`).WillReturnRows(sqlmock.NewRows(reportHdr).AddRow(8, reporter, "message", 3, "harassment", nil, "open", nil, created, nil, 1))
	mock.ExpectQuery(`FROM users u`).WillReturnRows(summaryRows(reporter))
	mock.ExpectQuery(`FROM messages WHERE id IN`).WillReturnRows(sqlmock.NewRows([]string{"id", "t", "x", "o", "c"}).AddRow(3, nil, "private words", 60, 4))
	expectAudit(mock, "moderation.read_messages")

	w := testutil.Request(r, http.MethodGet, "/v1/admin/reports", nil, nil)
	var items []AdminReport
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &items)
	if w.Code != http.StatusOK || len(items) != 1 || items[0].TargetPreview.Text == nil {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestAdmin_ListReportsValidation(t *testing.T) {
	_, _, r := adminSetup(t, nil, "reports.read")
	w := testutil.Request(r, http.MethodGet, "/v1/admin/reports?status=closed&reason=x", nil, nil)
	e := decode(t, w.Body.Bytes())
	if w.Code != http.StatusUnprocessableEntity || e.Error.Fields["status"] == "" || e.Error.Fields["reason"] == "" {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

var actionHdr = []string{"id", "admin_id", "name", "target_type", "target_id", "action", "reason", "report_id", "created_at"}

func TestAdmin_CreateActionUnhideConversation(t *testing.T) {
	_, mock, r := adminSetup(t, nil, "reports.resolve")
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM conversations WHERE id = \? FOR UPDATE`).WithArgs(4).WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("removed", 50, 0))
	mock.ExpectExec(`UPDATE conversations SET status = \? WHERE id = \?`).WithArgs("active", 4).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO moderation_actions`).WithArgs(adminID, "conversation", 4, "unhide", "appeal accepted", nil).WillReturnResult(sqlmock.NewResult(11, 1))
	expectAudit(mock, "moderation.unhide")
	mock.ExpectCommit()
	mock.ExpectQuery(`FROM moderation_actions a LEFT JOIN admin_users au ON au.id = a.admin_id WHERE a.id = \?`).WithArgs(11).
		WillReturnRows(sqlmock.NewRows(actionHdr).AddRow(11, adminID, "Mod One", "conversation", 4, "unhide", "appeal accepted", nil, created))

	w := testutil.Request(r, http.MethodPost, "/v1/admin/moderation/actions",
		map[string]any{"targetType": "conversation", "targetId": 4, "action": "unhide", "reason": "appeal accepted"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	want := `{"id":11,"admin":{"id":1,"name":"Mod One"},"targetType":"conversation","targetId":4,"action":"unhide","reason":"appeal accepted","reportId":null,"createdAt":"2026-11-12T09:30:00Z"}`
	if got := string(decode(t, w.Body.Bytes()).Data); got != want {
		t.Fatalf("got %s", got)
	}
}

func TestAdmin_CreateActionHideMessageClosesReports(t *testing.T) {
	_, mock, r := adminSetup(t, nil, "reports.resolve")
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM messages WHERE id = \? FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("visible", 60, 0))
	mock.ExpectExec(`UPDATE messages SET status = \?`).WithArgs("hidden", 3).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO moderation_actions`).WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectExec(`UPDATE reports SET status = 'actioned'`).WithArgs(adminID, "abuse", sqlmock.AnyArg(), "message", 3).WillReturnResult(sqlmock.NewResult(0, 1))
	expectAudit(mock, "moderation.hide")
	mock.ExpectCommit()
	mock.ExpectQuery(`FROM moderation_actions a`).WillReturnRows(sqlmock.NewRows(actionHdr).AddRow(12, adminID, "Mod One", "message", 3, "hide", "abuse", nil, created))

	w := testutil.Request(r, http.MethodPost, "/v1/admin/moderation/actions",
		map[string]any{"targetType": "message", "targetId": 3, "action": "hide", "reason": "abuse"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestAdmin_CreateActionErrors(t *testing.T) {
	t.Run("validation", func(t *testing.T) {
		_, _, r := adminSetup(t, nil, "reports.resolve")
		w := testutil.Request(r, http.MethodPost, "/v1/admin/moderation/actions", map[string]any{"targetType": "post", "targetId": 1, "action": "dismiss"}, nil)
		e := decode(t, w.Body.Bytes())
		if w.Code != http.StatusUnprocessableEntity || e.Error.Fields["action"] == "" || e.Error.Fields["reason"] == "" {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
	})
	t.Run("event unhide unsupported", func(t *testing.T) {
		_, mock, r := adminSetup(t, nil, "reports.resolve")
		mock.ExpectBegin()
		mock.ExpectQuery(`FROM events WHERE id = \? FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("removed", 77, 3))
		mock.ExpectRollback()
		w := testutil.Request(r, http.MethodPost, "/v1/admin/moderation/actions", map[string]any{"targetType": "event", "targetId": 9, "action": "unhide", "reason": "x"}, nil)
		if w.Code != http.StatusConflict {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
	})
	t.Run("missing target", func(t *testing.T) {
		_, mock, r := adminSetup(t, nil, "reports.resolve")
		mock.ExpectBegin()
		mock.ExpectQuery(`FROM posts WHERE id = \? FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}))
		mock.ExpectRollback()
		w := testutil.Request(r, http.MethodPost, "/v1/admin/moderation/actions", map[string]any{"targetType": "post", "targetId": 1, "action": "warn", "reason": "x"}, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
	})
	t.Run("system message has no owner", func(t *testing.T) {
		_, mock, r := adminSetup(t, nil, "reports.resolve")
		mock.ExpectBegin()
		mock.ExpectQuery(`FROM messages WHERE id = \? FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("visible", 0, 0))
		mock.ExpectRollback()
		w := testutil.Request(r, http.MethodPost, "/v1/admin/moderation/actions", map[string]any{"targetType": "message", "targetId": 1, "action": "suspend_user", "reason": "x"}, nil)
		if w.Code != http.StatusConflict {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
	})
}

func TestAdmin_ListActions(t *testing.T) {
	_, mock, r := adminSetup(t, nil, "reports.read")
	mock.ExpectQuery(`FROM moderation_actions a LEFT JOIN admin_users au ON au.id = a.admin_id WHERE 1 = 1 AND a.target_type = \? AND a.target_id = \? ORDER BY a.id DESC LIMIT \?`).
		WithArgs("post", 70, 21).
		WillReturnRows(sqlmock.NewRows(actionHdr).
			AddRow(2, adminID, "Mod One", "post", 70, "hide", "spam", 1, created).
			AddRow(1, nil, nil, "post", 70, "auto_hide", "Open reports reached 5", 1, created))

	w := testutil.Request(r, http.MethodGet, "/v1/admin/moderation/actions?targetType=post&targetId=70", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var items []ModerationAction
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &items)
	if len(items) != 2 || items[0].Admin == nil || items[1].Admin != nil || *items[1].ReportID != 1 {
		t.Fatalf("items %+v", items)
	}

	w = testutil.Request(r, http.MethodGet, "/v1/admin/moderation/actions?targetId=70", nil, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", w.Code)
	}
}

func TestCreateReport_CommunityTarget(t *testing.T) {
	_, mock, r := userSetup(t, map[string]any{"moderation.auto_hide_report_threshold": 1})
	mock.ExpectQuery(`FROM conversations c\s+LEFT JOIN conversation_members cm ON cm.conversation_id = c.id AND cm.user_id = \?\s+WHERE c.id = \? AND c.type = 'community' AND c.status = 'active'`).
		WithArgs(reporter, 51).WillReturnRows(count(1))
	mock.ExpectQuery(`SELECT id FROM reports WHERE reporter_id = \?`).WithArgs(reporter, "community", 51).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO reports`).WithArgs(reporter, "community", 51, "scam", nil).WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectCommit() // threshold 1, but communities are only queued: no count, no hide

	body := map[string]any{"targetType": "community", "targetId": 51, "reason": "scam"}
	if w := testutil.Request(r, http.MethodPost, "/v1/reports", body, nil); w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}

	// Not a community (or private and not a member) → 404.
	mock.ExpectQuery(`c.type = 'community'`).WithArgs(reporter, 52).WillReturnRows(count(0))
	body["targetId"] = 52
	if w := testutil.Request(r, http.MethodPost, "/v1/reports", body, nil); w.Code != http.StatusNotFound {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestCreateReport_RateLimitFromConfig(t *testing.T) {
	d, mock, r := userSetup(t, map[string]any{"moderation.report_rate_limit_per_hour": 3})
	if err := d.Redis.Set(context.Background(), "rl:report:42", 3, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	if w := testutil.Request(r, http.MethodPost, "/v1/reports", report, nil); w.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	// Below the configured limit the report goes through.
	if err := d.Redis.Set(context.Background(), "rl:report:42", 2, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	expectNewPostReport(mock, 1)
	mock.ExpectCommit()
	if w := testutil.Request(r, http.MethodPost, "/v1/reports", report, nil); w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestAdmin_ListReportsPreviewConversationIDs(t *testing.T) {
	_, mock, r := adminSetup(t, nil, "reports.read")
	mock.ExpectQuery(`FROM reports r`).WillReturnRows(sqlmock.NewRows(reportHdr).
		AddRow(8, reporter, "message", 3, "harassment", nil, "open", nil, created, nil, 1).
		AddRow(7, reporter, "community", 51, "scam", nil, "open", nil, created, nil, 1))
	mock.ExpectQuery(`FROM users u`).WillReturnRows(summaryRows(reporter))
	mock.ExpectQuery(`SELECT id, NULL, body, sender_id, conversation_id FROM messages WHERE id IN`).WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"id", "t", "x", "o", "c"}).AddRow(3, nil, "private words", 60, 4))
	mock.ExpectQuery(`SELECT id, title, description, owner_id, id FROM conversations WHERE type = 'community' AND id IN`).WithArgs(51).
		WillReturnRows(sqlmock.NewRows([]string{"id", "t", "x", "o", "c"}).AddRow(51, "Pune Sunday Football", "weekly games", 70, 51))

	w := testutil.Request(r, http.MethodGet, "/v1/admin/reports", nil, nil)
	var items []AdminReport
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &items)
	if w.Code != http.StatusOK || len(items) != 2 {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	if c := items[0].TargetPreview.ConversationID; c == nil || *c != 4 || items[0].TargetPreview.Text != nil {
		t.Fatalf("message preview %+v", items[0].TargetPreview)
	}
	if c := items[1].TargetPreview.ConversationID; c == nil || *c != 51 || *items[1].TargetPreview.Title != "Pune Sunday Football" {
		t.Fatalf("community preview %+v", items[1].TargetPreview)
	}
}

func TestAdmin_CreateActionHideCommunity(t *testing.T) {
	_, mock, r := adminSetup(t, nil, "reports.resolve")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, COALESCE\(owner_id, 0\), 0 FROM conversations WHERE id = \? AND type = 'community' FOR UPDATE`).WithArgs(51).
		WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}).AddRow("active", 70, 0))
	mock.ExpectExec(`UPDATE conversations SET status = \? WHERE id = \? AND type = 'community'`).WithArgs("removed", 51).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO moderation_actions`).WithArgs(adminID, "community", 51, "hide", "scam group", nil).WillReturnResult(sqlmock.NewResult(13, 1))
	mock.ExpectExec(`UPDATE reports SET status = 'actioned'`).WithArgs(adminID, "scam group", sqlmock.AnyArg(), "community", 51).WillReturnResult(sqlmock.NewResult(0, 1))
	expectAudit(mock, "moderation.hide")
	mock.ExpectCommit()
	mock.ExpectQuery(`FROM moderation_actions a LEFT JOIN admin_users au`).WithArgs(13).
		WillReturnRows(sqlmock.NewRows(actionHdr).AddRow(13, adminID, "Mod One", "community", 51, "hide", "scam group", nil, created))

	w := testutil.Request(r, http.MethodPost, "/v1/admin/moderation/actions",
		map[string]any{"targetType": "community", "targetId": 51, "action": "hide", "reason": "scam group"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}

	// A conversation id that is not a community → 404.
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM conversations WHERE id = \? AND type = 'community' FOR UPDATE`).WithArgs(4).WillReturnRows(sqlmock.NewRows([]string{"status", "owner", "sport"}))
	mock.ExpectRollback()
	w = testutil.Request(r, http.MethodPost, "/v1/admin/moderation/actions",
		map[string]any{"targetType": "community", "targetId": 4, "action": "hide", "reason": "x"}, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

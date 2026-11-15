package ads

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

var fixedNow = time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC) // 2026-11-13 01:30 IST

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

func deps(t *testing.T, cfg map[string]any) (*app.Deps, sqlmock.Sqlmock) {
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return fixedNow }
	return d, mock
}

func userEngine(d *app.Deps, u userdir.Basic) http.Handler {
	r, routes := testutil.UserEngine(u)
	New(d).Routes(routes)
	return r
}

func adminEngine(d *app.Deps, perms ...string) http.Handler {
	r, routes := testutil.AdminEngine(9, perms)
	New(d).Routes(routes)
	return r
}

var cols = []string{"id", "advertiser_name", "title", "body", "image_upload_id", "target_url", "placement", "sport_id", "state", "priority", "starts_at", "ends_at", "status"}

func campaignRow(rows *sqlmock.Rows, id int64, img any) *sqlmock.Rows {
	return rows.AddRow(id, "Acme Sports", "Bats sale", nil, img, "https://acme.test/sale", "home_feed", nil, nil, 0,
		fixedNow.Add(-time.Hour), fixedNow.Add(24*time.Hour), "active")
}

func TestServeDisabledReturnsEmpty(t *testing.T) {
	d, _ := deps(t, nil) // ads.enabled defaults to false: no SQL at all
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(1)), "GET", "/v1/ads?placement=home_feed", nil, nil)
	if w.Code != 200 || string(decode(t, w.Body.Bytes()).Data) != "[]" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestServeAdultGetsContextualAds(t *testing.T) {
	d, mock := deps(t, map[string]any{"ads.enabled": true})
	mock.ExpectQuery(`SELECT .* FROM ad_campaigns\s+WHERE status = 'active' AND starts_at <= \? AND ends_at > \? AND placement = \?.*sport_id IS NULL OR sport_id = \?.*state IS NULL OR state = \?.*ORDER BY priority DESC, id DESC LIMIT \?`).
		WithArgs(fixedNow, fixedNow, "events_list", int64(4), "Maharashtra", 3).
		WillReturnRows(campaignRow(sqlmock.NewRows(cols), 5, int64(77)))
	mock.ExpectQuery(`SELECT id, s3_key FROM media_uploads`).WithArgs(int64(77)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}).AddRow(77, "post_image/0/x.jpg"))
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(1)), "GET", "/v1/ads?placement=events_list&sportId=4&state=Maharashtra", nil, nil)
	if w.Code != 200 {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
	var out []Ad
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &out)
	if len(out) != 1 || out[0].Label != "Sponsored" || out[0].ImageURL == nil || out[0].Body != nil {
		t.Fatalf("unexpected %s", w.Body)
	}
}

func TestServeNoContextMatchesOnlyUntargeted(t *testing.T) {
	d, mock := deps(t, map[string]any{"ads.enabled": true})
	mock.ExpectQuery(`FROM ad_campaigns`).WithArgs(fixedNow, fixedNow, "search", nil, nil, 3).
		WillReturnRows(sqlmock.NewRows(cols))
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(1)), "GET", "/v1/ads?placement=search", nil, nil)
	if w.Code != 200 || string(decode(t, w.Body.Bytes()).Data) != "[]" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestServeMinorRule(t *testing.T) {
	minor := testutil.VerifiedUser(2)
	minor.IsMinor = true

	d, _ := deps(t, map[string]any{"ads.enabled": true})
	w := testutil.Request(userEngine(d, minor), "GET", "/v1/ads?placement=home_feed", nil, nil)
	if w.Code != 200 || string(decode(t, w.Body.Bytes()).Data) != "[]" {
		t.Fatalf("minor saw ads: %d %s", w.Code, w.Body)
	}

	d2, mock := deps(t, map[string]any{"ads.enabled": true, "ads.show_to_minors": true})
	mock.ExpectQuery(`FROM ad_campaigns`).WillReturnRows(campaignRow(sqlmock.NewRows(cols), 5, nil))
	w = testutil.Request(userEngine(d2, minor), "GET", "/v1/ads?placement=home_feed", nil, nil)
	var out []Ad
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &out)
	if w.Code != 200 || len(out) != 1 {
		t.Fatalf("show_to_minors=true should serve: %d %s", w.Code, w.Body)
	}
}

func TestServeBadPlacement(t *testing.T) {
	d, _ := deps(t, map[string]any{"ads.enabled": true})
	for _, q := range []string{"", "?placement=sidebar"} {
		w := testutil.Request(userEngine(d, testutil.VerifiedUser(1)), "GET", "/v1/ads"+q, nil, nil)
		if w.Code != 422 || decode(t, w.Body.Bytes()).Error.Fields["placement"] == "" {
			t.Fatalf("%q: got %d %s", q, w.Code, w.Body)
		}
	}
}

func TestImpressionAndClick(t *testing.T) {
	d, mock := deps(t, map[string]any{"ads.enabled": true})
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ad_campaigns WHERE id = \? AND status = 'active'`).WithArgs(int64(5), fixedNow, fixedNow).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO ad_daily_stats .* VALUES \(\?, \?, 1, 0\)\s+ON DUPLICATE KEY UPDATE impressions = impressions \+ 1`).
		WithArgs(int64(5), "2026-11-13"). // IST date, not the UTC one
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ad_campaigns`).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectExec(`ON DUPLICATE KEY UPDATE clicks = clicks \+ 1`).WithArgs(int64(5), "2026-11-13").
		WillReturnResult(sqlmock.NewResult(0, 1))
	h := userEngine(d, testutil.VerifiedUser(1))
	if w := testutil.Request(h, "POST", "/v1/ads/5/impression", nil, nil); w.Code != 204 {
		t.Fatalf("impression %d %s", w.Code, w.Body)
	}
	if w := testutil.Request(h, "POST", "/v1/ads/5/click", nil, nil); w.Code != 204 {
		t.Fatalf("click %d %s", w.Code, w.Body)
	}
}

func TestImpressionNotServable(t *testing.T) {
	d, mock := deps(t, map[string]any{"ads.enabled": true})
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ad_campaigns`).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	w := testutil.Request(userEngine(d, testutil.VerifiedUser(1)), "POST", "/v1/ads/5/impression", nil, nil)
	if w.Code != 404 {
		t.Fatalf("got %d", w.Code)
	}
	// Ads off: 404 without touching the DB.
	d2, _ := deps(t, nil)
	if w := testutil.Request(userEngine(d2, testutil.VerifiedUser(1)), "POST", "/v1/ads/5/click", nil, nil); w.Code != 404 {
		t.Fatalf("disabled: got %d", w.Code)
	}
	// Minor without show_to_minors: 404.
	minor := testutil.VerifiedUser(2)
	minor.IsMinor = true
	d3, _ := deps(t, map[string]any{"ads.enabled": true})
	if w := testutil.Request(userEngine(d3, minor), "POST", "/v1/ads/5/click", nil, nil); w.Code != 404 {
		t.Fatalf("minor: got %d", w.Code)
	}
}

func TestAdminNeedsPermission(t *testing.T) {
	d, _ := deps(t, nil)
	h := adminEngine(d, "reports.read")
	for _, rq := range [][2]string{{"GET", "/v1/admin/ads"}, {"POST", "/v1/admin/ads"}, {"PUT", "/v1/admin/ads/1"}, {"GET", "/v1/admin/ads/1/stats"}} {
		if w := testutil.Request(h, rq[0], rq[1], map[string]any{}, nil); w.Code != 403 {
			t.Fatalf("%v: got %d", rq, w.Code)
		}
	}
}

func validInput() map[string]any {
	return map[string]any{
		"advertiserName": "Acme Sports", "title": "Bats sale", "targetUrl": "https://acme.test/sale",
		"placement": "home_feed", "startsAt": "2026-11-13T00:00:00Z", "endsAt": "2026-11-20T00:00:00Z",
	}
}

func TestAdminCreateValidation(t *testing.T) {
	d, _ := deps(t, nil)
	h := adminEngine(d, "ads.write")
	w := testutil.Request(h, "POST", "/v1/admin/ads", map[string]any{"title": "x"}, nil)
	if w.Code != 422 || decode(t, w.Body.Bytes()).Error.Fields["advertiserName"] != "is required" {
		t.Fatalf("missing fields: %d %s", w.Code, w.Body)
	}
	cases := map[string]map[string]any{
		"targetUrl": {"targetUrl": "http://acme.test"},
		"placement": {"placement": "sidebar"},
		"endsAt":    {"endsAt": "2026-11-12T00:00:00Z"},
		"status":    {"status": "live"},
		"body":      {"body": string(make([]rune, 301))},
	}
	for field, patch := range cases {
		in := validInput()
		for k, v := range patch {
			in[k] = v
		}
		mock := sqlmockFor(t, d)
		mock.ExpectBegin()
		mock.ExpectRollback()
		w := testutil.Request(h, "POST", "/v1/admin/ads", in, nil)
		if w.Code != 422 || decode(t, w.Body.Bytes()).Error.Fields[field] == "" {
			t.Fatalf("%s: got %d %s", field, w.Code, w.Body)
		}
	}
}

// sqlmockFor gives each sub-case a fresh DB so expectations do not leak.
func sqlmockFor(t *testing.T, d *app.Deps) sqlmock.Sqlmock {
	fresh, mock := deps(t, nil)
	d.DB, d.Media = fresh.DB, fresh.Media
	return mock
}

func TestAdminCreateWithImage(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, user_id, purpose, s3_key, content_type, size_bytes, status FROM media_uploads WHERE id = \? AND user_id = \?`).
		WithArgs(int64(77), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "purpose", "s3_key", "content_type", "size_bytes", "status"}).
			AddRow(77, 0, "post_image", "post_image/0/x.jpg", "image/jpeg", 100, "confirmed"))
	mock.ExpectExec(`UPDATE media_uploads SET status = 'attached'`).WithArgs(int64(77), int64(0)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO ad_campaigns`).
		WithArgs("Acme Sports", "Bats sale", nil, int64(77), "https://acme.test/sale", "home_feed", nil, nil, 0,
			time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), "draft", int64(9)).
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(int64(9), "ads.create", "ad_campaign", "12", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .* FROM ad_campaigns WHERE id = \?`).WithArgs(int64(12)).
		WillReturnRows(campaignRow(sqlmock.NewRows(cols), 12, int64(77)))
	mock.ExpectQuery(`SELECT id, s3_key FROM media_uploads`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}).AddRow(77, "post_image/0/x.jpg"))
	in := validInput()
	in["imageUploadId"] = 77
	w := testutil.Request(adminEngine(d, "ads.write"), "POST", "/v1/admin/ads", in, nil)
	if w.Code != 200 {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
	var c Campaign
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &c)
	if c.ID != 12 || c.ImageURL == nil || *c.ImageUploadID != 77 {
		t.Fatalf("unexpected %s", w.Body)
	}
}

func TestAdminCreateForeignImageRejected(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM media_uploads WHERE id = \? AND user_id = \?`).WithArgs(int64(78), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	in := validInput()
	in["imageUploadId"] = 78
	w := testutil.Request(adminEngine(d, "ads.write"), "POST", "/v1/admin/ads", in, nil)
	if w.Code != 422 || decode(t, w.Body.Bytes()).Error.Fields["imageUploadId"] == "" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestAdminUpdate(t *testing.T) {
	d, mock := deps(t, nil)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM ad_campaigns WHERE id = \? FOR UPDATE`).WithArgs(int64(404)).WillReturnRows(sqlmock.NewRows(cols))
	mock.ExpectRollback()
	h := adminEngine(d, "ads.write")
	if w := testutil.Request(h, "PUT", "/v1/admin/ads/404", map[string]any{"status": "paused"}, nil); w.Code != 404 {
		t.Fatalf("missing: got %d", w.Code)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WithArgs(int64(5)).WillReturnRows(campaignRow(sqlmock.NewRows(cols), 5, nil))
	mock.ExpectExec(`UPDATE ad_campaigns SET`).
		WithArgs("Acme Sports", "Bats sale", "New copy", nil, "https://acme.test/sale", "home_feed", nil, nil, 0,
			fixedNow.Add(-time.Hour), fixedNow.Add(24*time.Hour), "paused", int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(int64(9), "ads.update", "ad_campaign", "5", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`FROM ad_campaigns WHERE id = \?`).WithArgs(int64(5)).WillReturnRows(campaignRow(sqlmock.NewRows(cols), 5, nil))
	w := testutil.Request(h, "PUT", "/v1/admin/ads/5", map[string]any{"status": "paused", "body": "New copy"}, nil)
	if w.Code != 200 {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestAdminListPaging(t *testing.T) {
	d, mock := deps(t, nil)
	rows := sqlmock.NewRows(cols)
	campaignRow(rows, 9, nil)
	campaignRow(rows, 8, nil)
	campaignRow(rows, 7, nil)
	mock.ExpectQuery(`FROM ad_campaigns\s+WHERE \(\? = '' OR status = \?\) AND \(\? = 0 OR id < \?\)\s+ORDER BY id DESC LIMIT \?`).
		WithArgs("active", "active", int64(0), int64(0), 3).WillReturnRows(rows)
	h := adminEngine(d, "ads.write")
	w := testutil.Request(h, "GET", "/v1/admin/ads?status=active&limit=2", nil, nil)
	e := decode(t, w.Body.Bytes())
	if w.Code != 200 || e.Meta["nextCursor"] == nil {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
	if w := testutil.Request(h, "GET", "/v1/admin/ads?status=bogus", nil, nil); w.Code != 422 {
		t.Fatalf("bad status: %d", w.Code)
	}
}

func TestAdminStats(t *testing.T) {
	d, mock := deps(t, nil)
	h := adminEngine(d, "ads.write")
	for _, q := range []string{"?from=2026-13-01", "?from=2026-11-10&to=2026-11-01", "?from=2025-01-01&to=2026-11-01"} {
		if w := testutil.Request(h, "GET", "/v1/admin/ads/5/stats"+q, nil, nil); w.Code != 422 {
			t.Fatalf("%s: got %d", q, w.Code)
		}
	}
	mock.ExpectQuery(`FROM ad_campaigns WHERE id = \?`).WithArgs(int64(5)).WillReturnRows(campaignRow(sqlmock.NewRows(cols), 5, nil))
	mock.ExpectQuery(`FROM ad_daily_stats\s+WHERE ad_id = \? AND stat_date BETWEEN \? AND \?`).WithArgs(int64(5), "2026-11-01", "2026-11-03").
		WillReturnRows(sqlmock.NewRows([]string{"d", "i", "c"}).AddRow("2026-11-02", 40, 3))
	w := testutil.Request(h, "GET", "/v1/admin/ads/5/stats?from=2026-11-01&to=2026-11-03", nil, nil)
	var out []DayStat
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &out)
	if w.Code != 200 || len(out) != 3 || out[1].Impressions != 40 || out[0].Impressions != 0 {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
	// Default range: last 30 IST days ending today (2026-11-13).
	mock.ExpectQuery(`FROM ad_campaigns WHERE id = \?`).WillReturnRows(campaignRow(sqlmock.NewRows(cols), 5, nil))
	mock.ExpectQuery(`FROM ad_daily_stats`).WithArgs(int64(5), "2026-10-15", "2026-11-13").WillReturnRows(sqlmock.NewRows([]string{"d", "i", "c"}))
	w = testutil.Request(h, "GET", "/v1/admin/ads/5/stats", nil, nil)
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &out)
	if w.Code != 200 || len(out) != 30 {
		t.Fatalf("default: %d len=%d", w.Code, len(out))
	}
	mock.ExpectQuery(`FROM ad_campaigns WHERE id = \?`).WillReturnRows(sqlmock.NewRows(cols))
	if w := testutil.Request(h, "GET", "/v1/admin/ads/6/stats", nil, nil); w.Code != 404 {
		t.Fatalf("unknown: %d", w.Code)
	}
}

func TestTargetURL(t *testing.T) {
	for v, ok := range map[string]bool{"https://a.test/x": true, "http://a.test": false, "/relative": false, "https://user@a.test": false, "javascript:alert(1)": false} {
		if (checkTargetURL(v) == "") != ok {
			t.Errorf("%q: want ok=%v", v, ok)
		}
	}
}

func TestEndCampaignsJob(t *testing.T) {
	d, mock := deps(t, nil)
	job := New(d).Jobs()["ads.end_campaigns"]
	if job == nil {
		t.Fatal("job not registered")
	}
	mock.ExpectExec(`UPDATE ad_campaigns SET status = 'ended'\s+WHERE status IN \('active', 'paused'\) AND ends_at <= \? ORDER BY id LIMIT \?`).
		WithArgs(fixedNow, 500).WillReturnResult(sqlmock.NewResult(0, 2))
	if err := job(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Second run: nothing left to end (idempotent).
	mock.ExpectExec(`UPDATE ad_campaigns SET status = 'ended'`).WithArgs(fixedNow, 500).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := job(context.Background()); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`UPDATE ad_campaigns SET status = 'ended'`).WillReturnError(errors.New("db down"))
	if err := job(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

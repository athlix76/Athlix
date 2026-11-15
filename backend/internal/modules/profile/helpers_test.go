package profile

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

const (
	me    = int64(7)
	other = int64(9)
)

// 06:00 UTC is 11:30 IST on the same day.
var fixedNow = time.Date(2026, 6, 15, 6, 0, 0, 0, time.UTC)

const today = "2026-06-15"

func q(s string) string { return regexp.QuoteMeta(s) }

type body struct {
	Data  json.RawMessage `json:"data"`
	Meta  map[string]any  `json:"meta"`
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

func status(t *testing.T, w *httptest.ResponseRecorder, want int) body {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body)
	}
	return decode(t, w)
}

type env struct {
	d    *app.Deps
	mock sqlmock.Sqlmock
	m    *Module
	r    *gin.Engine
	me   userdir.Basic
}

func newEnv(t *testing.T, cfg map[string]any, viewer userdir.Basic) *env {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return fixedNow }
	e := &env{d: d, mock: mock, m: New(d), me: viewer}
	r, routes := testutil.UserEngine(viewer)
	e.m.Routes(routes)
	e.r = r
	return e
}

func adult() userdir.Basic { return testutil.VerifiedUser(me) }

func minor() userdir.Basic {
	u := testutil.VerifiedUser(me)
	u.IsMinor, u.GuardianConsentStatus = true, "granted"
	return u
}

func (e *env) do(method, path string, in any) *httptest.ResponseRecorder {
	return testutil.Request(e.r, method, "/v1"+path, in, nil)
}

// ---- SQL expectations ----

// expectBasic expects userdir.Get for b.
func (e *env) expectBasic(b userdir.Basic) {
	e.mock.ExpectQuery(q(`SELECT id, display_name, primary_role, status, onboarding_step, verification_status, is_minor, guardian_consent_status FROM users WHERE id = ?`)).
		WithArgs(b.ID).WillReturnRows(sqlmock.NewRows([]string{"id", "display_name", "primary_role", "status", "onboarding_step", "verification_status", "is_minor", "guardian_consent_status"}).
		AddRow(b.ID, b.DisplayName, b.PrimaryRole, b.Status, b.OnboardingStep, b.VerificationStatus, b.IsMinor, b.GuardianConsentStatus))
	rows := sqlmock.NewRows([]string{"role"})
	for _, r := range b.Roles {
		rows.AddRow(r)
	}
	e.mock.ExpectQuery(q(`SELECT role FROM user_roles WHERE user_id = ?`) + `$`).WithArgs(b.ID).WillReturnRows(rows)
}

func (e *env) expectBasicMissing(id int64) {
	e.mock.ExpectQuery(q(`SELECT id, display_name, primary_role, status`)).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

func (e *env) expectBlocked(a, b int64, blocked bool) {
	n := 0
	if blocked {
		n = 1
	}
	e.mock.ExpectQuery(q(`SELECT COUNT(*) FROM blocks WHERE (blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?)`)).
		WithArgs(a, b, b, a).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func (e *env) expectFollows(follower, followee int64, yes bool) {
	n := 0
	if yes {
		n = 1
	}
	e.mock.ExpectQuery(q(`SELECT COUNT(*) FROM follows WHERE follower_id = ? AND followee_id = ?`)).
		WithArgs(follower, followee).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

// prow is a profiles row; zero values are NULL / defaults.
type prow struct {
	Bio, City, District, State, Academy any
	SportID, PositionID, Photo          any
	BirthYear, CoachingSince            any
	Visibility, StatsVisibility         string
	Followers, Following                int
}

var profileColumns = []string{"bio", "sport_id", "position_id", "city", "district", "state", "academy_club", "photo_upload_id", "birth_year", "visibility", "stats_visibility", "coaching_since_year", "followers_count", "following_count"}

func (e *env) expectProfile(uid int64, p *prow, forUpdate bool) {
	query := q(`SELECT ` + profileCols + ` FROM profiles WHERE user_id = ?`)
	if forUpdate {
		query += ` FOR UPDATE`
	} else {
		query += `$`
	}
	rows := sqlmock.NewRows(profileColumns)
	if p != nil {
		if p.Visibility == "" {
			p.Visibility = "public"
		}
		if p.StatsVisibility == "" {
			p.StatsVisibility = "public"
		}
		rows.AddRow(p.Bio, p.SportID, p.PositionID, p.City, p.District, p.State, p.Academy, p.Photo, p.BirthYear, p.Visibility, p.StatsVisibility, p.CoachingSince, p.Followers, p.Following)
	}
	e.mock.ExpectQuery(query).WithArgs(uid).WillReturnRows(rows)
}

// expectSummary expects userdir.Summary for uid (no photo).
func (e *env) expectSummary(uid int64, name string, city any) {
	e.mock.ExpectQuery(q(`SELECT u.id, COALESCE(u.display_name,'')`)).WithArgs(uid).
		WillReturnRows(sqlmock.NewRows([]string{"id", "display_name", "primary_role", "verification_status", "photo_upload_id", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"}).
			AddRow(uid, name, "player", "verified", nil, city, "Maharashtra", 1, "cricket", "Cricket", 2, "batter", "Batter"))
}

func (e *env) expectView(viewed, viewer int64) {
	e.mock.ExpectExec(q(`INSERT IGNORE INTO profile_views (viewed_id, viewer_id, viewed_on) VALUES (?, ?, ?)`)).
		WithArgs(viewed, viewer, today).WillReturnResult(sqlmock.NewResult(0, 1))
}

func (e *env) expectSports(uid int64) {
	e.mock.ExpectQuery(q(`SELECT s.id, s.code, s.name, sp.id, sp.code, sp.name, ps.is_primary FROM profile_sports ps`)).WithArgs(uid).
		WillReturnRows(sqlmock.NewRows([]string{"sid", "scode", "sname", "pid", "pcode", "pname", "primary"}).
			AddRow(1, "cricket", "Cricket", 2, "batter", "Batter", true).
			AddRow(3, "football", "Football", nil, nil, nil, false))
}

func (e *env) expectBenefits(uid int64, keys ...string) {
	rows := sqlmock.NewRows([]string{"benefit_key", "value"})
	for _, k := range keys {
		rows.AddRow(k, []byte("true"))
	}
	e.mock.ExpectQuery(q(`SELECT b.benefit_key, b.value`)).WithArgs(uid).WillReturnRows(rows)
}

func (e *env) expectPrivacy(uid int64, allow string) {
	e.mock.ExpectQuery(q(`SELECT notification_prefs, privacy FROM user_settings WHERE user_id = ?`)).WithArgs(uid).
		WillReturnRows(sqlmock.NewRows([]string{"notification_prefs", "privacy"}).AddRow([]byte(`{}`), []byte(`{"allowDmsFrom":"`+allow+`","showInSearch":true}`)))
}

func (e *env) expectConsultation(a, b int64, accepted bool) {
	n := 0
	if accepted {
		n = 1
	}
	e.mock.ExpectQuery(q(`SELECT COUNT(*) FROM consultation_requests WHERE status = 'accepted'`)).WithArgs(a, b, b, a).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

// expectUpload expects platform/media's owner-scoped upload lookup.
func (e *env) expectUpload(id, owner int64, purpose, status string) {
	rows := sqlmock.NewRows([]string{"id", "user_id", "purpose", "s3_key", "content_type", "size_bytes", "status"})
	if purpose != "" {
		rows.AddRow(id, owner, purpose, purpose+"/7/x.jpg", "image/jpeg", 100, status)
	}
	e.mock.ExpectQuery(q(`SELECT id, user_id, purpose, s3_key, content_type, size_bytes, status FROM media_uploads WHERE id = ? AND user_id = ?`)).
		WithArgs(id, owner).WillReturnRows(rows)
}

func (e *env) expectClaim(id, owner int64, purpose string) {
	e.expectUpload(id, owner, purpose, "confirmed")
	e.mock.ExpectExec(q(`UPDATE media_uploads SET status = 'attached'`)).WithArgs(id, owner).WillReturnResult(sqlmock.NewResult(0, 1))
}

func (e *env) expectPurge(id int64, key string) {
	e.mock.ExpectQuery(q(`SELECT s3_key, status FROM media_uploads WHERE id = ?`)).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"s3_key", "status"}).AddRow(key, "attached"))
	e.mock.ExpectExec(q(`UPDATE media_uploads SET status = 'deleted' WHERE id = ?`)).WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
}

func (e *env) expectViewURL(id int64, key string) {
	e.mock.ExpectQuery(q(`SELECT id, s3_key FROM media_uploads WHERE status IN ('confirmed','attached')`)).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}).AddRow(id, key))
}

func (e *env) expectActiveSport(id int64, ok bool) {
	n := 0
	if ok {
		n = 1
	}
	e.mock.ExpectQuery(q(`SELECT COUNT(*) FROM sports WHERE id = ? AND active = 1`)).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func (e *env) expectActivePosition(sportID, posID int64, ok bool) {
	n := 0
	if ok {
		n = 1
	}
	e.mock.ExpectQuery(q(`SELECT COUNT(*) FROM sport_positions WHERE id = ? AND sport_id = ? AND active = 1`)).WithArgs(posID, sportID).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

// anyOf matches an argument by a predicate.
type anyOf func(driver.Value) bool

func (f anyOf) Match(v driver.Value) bool { return f(v) }

func contains(s string, subs ...string) bool {
	for _, x := range subs {
		if !strings.Contains(s, x) {
			return false
		}
	}
	return true
}

func (e *env) storagePut(key string) { e.d.Storage.(*testutil.MemStorage).Put(key, "image/jpeg", 10) }

func (e *env) storageHas(key string) bool {
	_, err := e.d.Storage.(*testutil.MemStorage).Head(context.Background(), key)
	return err == nil
}

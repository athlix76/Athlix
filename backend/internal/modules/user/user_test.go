package user

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/user/usertest"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/requirements"
	"athlix/backend/internal/testutil"
)

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

func expectStatus(t *testing.T, w *httptest.ResponseRecorder, want int) body {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body.String())
	}
	return decode(t, w)
}

func userSetup(t *testing.T, cfg map[string]any) (*app.Deps, sqlmock.Sqlmock, *gin.Engine) {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	r, rt := testutil.UserEngine(testutil.VerifiedUser(42))
	New(d).Routes(rt)
	return d, mock, r
}

func adminSetup(t *testing.T, perms ...string) (*app.Deps, sqlmock.Sqlmock, *gin.Engine) {
	t.Helper()
	d, mock := testutil.Deps(t, nil)
	r, rt := testutil.AdminEngine(7, perms)
	New(d).Routes(rt)
	return d, mock, r
}

func TestGetMe(t *testing.T) {
	_, mock, r := userSetup(t, nil)
	usertest.ExpectMe(mock, usertest.MeRow{ID: 42, DisplayName: "Asha", PrimaryRole: "player", Roles: []string{"player"},
		Pending: [][]driverValue{{int64(3), "terms", 2, "Terms of Use"}}})
	b := expectStatus(t, testutil.Request(r, "GET", "/v1/me", nil, nil), 200)
	var m Me
	_ = json.Unmarshal(b.Data, &m)
	if m.ID != 42 || m.MobileMasked != "******3210" || *m.DisplayName != "Asha" || len(m.PendingPolicies) != 1 || m.PhotoURL != nil {
		t.Fatalf("me = %+v", m)
	}
	if _, err := uuid.Parse(m.StoreAccountToken); err != nil {
		t.Fatalf("storeAccountToken %q", m.StoreAccountToken)
	}
	var raw map[string]any
	_ = json.Unmarshal(b.Data, &raw)
	if _, ok := raw["photoUrl"]; !ok {
		t.Fatal("photoUrl must be present as null")
	}
}

func TestGetMeDeletedIs404(t *testing.T) {
	_, mock, r := userSetup(t, nil)
	mock.ExpectQuery(`FROM users u LEFT JOIN profiles`).WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	b := expectStatus(t, testutil.Request(r, "GET", "/v1/me", nil, nil), 404)
	if b.Error.Code != "NOT_FOUND" {
		t.Fatal(b.Error.Code)
	}
}

func TestStoreAccountTokenStable(t *testing.T) {
	d, _ := testutil.Deps(t, nil)
	a, b := StoreAccountToken(d.Crypto, 1), StoreAccountToken(d.Crypto, 1)
	u, err := uuid.Parse(a)
	if a != b || err != nil || u.Version() != 4 || StoreAccountToken(d.Crypto, 2) == a {
		t.Fatalf("token %s %s %v", a, b, err)
	}
}

func TestSettingsDefaultsAndPartialUpdate(t *testing.T) {
	_, mock, r := userSetup(t, nil)
	mock.ExpectQuery(`SELECT notification_prefs, privacy FROM user_settings WHERE user_id = \?`).WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"a", "b"}))
	b := expectStatus(t, testutil.Request(r, "GET", "/v1/me/settings", nil, nil), 200)
	var s Settings
	_ = json.Unmarshal(b.Data, &s)
	if s != DefaultSettings() {
		t.Fatalf("defaults %+v", s)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT notification_prefs, privacy FROM user_settings`).WithArgs(42).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b"}).AddRow(`{"messages":false,"events":true,"social":true,"marketing":false}`, `{"allowDmsFrom":"followers","showInSearch":true}`))
	mock.ExpectExec(`INSERT INTO user_settings`).
		WithArgs(42, `{"messages":false,"events":true,"social":true,"marketing":true}`, `{"allowDmsFrom":"mutual","showInSearch":true}`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	b = expectStatus(t, testutil.Request(r, "PUT", "/v1/me/settings", map[string]any{
		"notificationPrefs": map[string]any{"marketing": true},
		"privacy":           map[string]any{"allowDmsFrom": "mutual"},
	}, nil), 200)
	_ = json.Unmarshal(b.Data, &s)
	if s.NotificationPrefs.Messages || !s.NotificationPrefs.Marketing || s.Privacy.AllowDmsFrom != "mutual" {
		t.Fatalf("merged %+v", s)
	}
}

func TestSettingsValidation(t *testing.T) {
	_, _, r := userSetup(t, nil)
	b := expectStatus(t, testutil.Request(r, "PUT", "/v1/me/settings", map[string]any{"privacy": map[string]any{"allowDmsFrom": "strangers"}}, nil), 422)
	if b.Error.Fields["privacy.allowDmsFrom"] == "" {
		t.Fatal(b.Error.Fields)
	}
}

func TestAcceptPolicies(t *testing.T) {
	_, mock, r := userSetup(t, nil)
	expectStatus(t, testutil.Request(r, "POST", "/v1/me/policies/accept", map[string]any{"policyDocumentIds": []int{}}, nil), 422)

	mock.ExpectQuery(`SELECT id FROM policy_documents WHERE active = 1 AND id IN \(\?,\s*\?\)`).WithArgs(1, 9).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	expectStatus(t, testutil.Request(r, "POST", "/v1/me/policies/accept", map[string]any{"policyDocumentIds": []int{1, 9}}, nil), 422)

	mock.ExpectQuery(`SELECT id FROM policy_documents`).WithArgs(1, 2).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT IGNORE INTO policy_acceptances`).WithArgs(42, 1).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT IGNORE INTO policy_acceptances`).WithArgs(42, 2).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	usertest.ExpectMe(mock, usertest.MeRow{ID: 42, PrimaryRole: "player", Roles: []string{"player"}})
	expectStatus(t, testutil.Request(r, "POST", "/v1/me/policies/accept", map[string]any{"policyDocumentIds": []int{1, 2, 2}}, nil), 200)
}

func TestAddRole(t *testing.T) {
	_, mock, r := userSetup(t, nil)
	expectStatus(t, testutil.Request(r, "POST", "/v1/me/roles", map[string]any{"role": "admin"}, nil), 422)
	mock.ExpectExec(`INSERT IGNORE INTO user_roles \(user_id, role\) VALUES \(\?, \?\)`).WithArgs(42, "coach").WillReturnResult(sqlmock.NewResult(0, 1))
	usertest.ExpectMe(mock, usertest.MeRow{ID: 42, PrimaryRole: "player", Roles: []string{"coach", "player"}})
	b := expectStatus(t, testutil.Request(r, "POST", "/v1/me/roles", map[string]any{"role": "coach"}, nil), 200)
	var m Me
	_ = json.Unmarshal(b.Data, &m)
	if len(m.Roles) != 2 {
		t.Fatal(m.Roles)
	}
}

func TestChooseRole(t *testing.T) {
	_, mock, r := userSetup(t, nil)
	b := expectStatus(t, testutil.Request(r, "POST", "/v1/onboarding/role", map[string]any{"role": "x", "additionalRoles": []string{"y"}}, nil), 422)
	if b.Error.Fields["role"] == "" || b.Error.Fields["additionalRoles"] == "" {
		t.Fatal(b.Error.Fields)
	}

	// wrong step
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT onboarding_step FROM users WHERE id = \? AND status <> 'deleted' FOR UPDATE`).WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"s"}).AddRow("verification"))
	mock.ExpectRollback()
	b = expectStatus(t, testutil.Request(r, "POST", "/v1/onboarding/role", map[string]any{"role": "player"}, nil), 409)
	if b.Error.Code != "INVALID_STATE" {
		t.Fatal(b.Error.Code)
	}

	// happy: player + coach, moves to verification
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT onboarding_step FROM users`).WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"s"}).AddRow("role"))
	mock.ExpectExec(`UPDATE users SET primary_role = \? WHERE id = \?`).WithArgs("player", 42).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT IGNORE INTO user_roles`).WithArgs(42, "player").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT IGNORE INTO user_roles`).WithArgs(42, "coach").WillReturnResult(sqlmock.NewResult(0, 1))
	usertest.ExpectAdvance(mock, 42, usertest.AdvanceRow{PrimaryRole: "player", Step: "role", Verify: "unverified", Guardian: "not_required", Roles: []string{"coach", "player"}}, StepVerification)
	mock.ExpectCommit()
	usertest.ExpectMe(mock, usertest.MeRow{ID: 42, PrimaryRole: "player", Step: "verification", Verify: "unverified", Roles: []string{"coach", "player"}})
	b = expectStatus(t, testutil.Request(r, "POST", "/v1/onboarding/role", map[string]any{"role": "player", "additionalRoles": []string{"coach", "player"}}, nil), 200)
	var m Me
	_ = json.Unmarshal(b.Data, &m)
	if m.OnboardingStep != StepVerification {
		t.Fatal(m.OnboardingStep)
	}
}

func TestNextStepFollowsRequirements(t *testing.T) {
	ctx := context.Background()
	def := requirements.NewStatic(testutil.DefaultRequirements)
	cases := []struct {
		name     string
		req      *requirements.Checker
		role     string
		verify   string
		minor    bool
		guardian string
		profile  bool
		want     string
	}{
		{"no role", def, "", "unverified", false, "not_required", false, StepRole},
		{"needs digilocker", def, "player", "unverified", false, "not_required", false, StepVerification},
		{"minor needs guardian", def, "player", "verified", true, "pending", false, StepGuardian},
		{"minor granted", def, "player", "verified", true, "granted", false, StepProfile},
		{"adult no profile", def, "player", "verified", false, "not_required", false, StepProfile},
		{"done", def, "coach", "verified", false, "not_required", true, StepDone},
		{"digilocker off for player", requirements.NewStatic(map[string]bool{"player/digilocker/app_access": false}), "player", "unverified", false, "not_required", false, StepProfile},
		{"guardian off", requirements.NewStatic(map[string]bool{"player/digilocker/app_access": true}), "player", "verified", true, "pending", true, StepDone},
	}
	for _, c := range cases {
		if got := NextStep(ctx, c.req, c.role, nil, c.verify, c.minor, c.guardian, c.profile); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestAdvanceNeverRegressesFromDone(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	usertest.ExpectAdvance(mock, 5, usertest.AdvanceRow{PrimaryRole: "player", Step: "done", Verify: "unverified"}, StepDone)
	step, err := Advance(context.Background(), d, d.DB, 5)
	if err != nil || step != StepDone {
		t.Fatal(step, err)
	}
}

func TestDeleteMe(t *testing.T) {
	d, mock, r := userSetup(t, nil)
	got := 0
	d.Dispatcher.On(events.UserDeleted, func(_ context.Context, e events.Event) error {
		var p events.UserPayload
		_ = e.Decode(&p)
		if p.UserID == 42 {
			got++
		}
		return nil
	})
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET status = 'deleted'`).WithArgs([]byte{}, sqlmock.AnyArg(), sqlmock.AnyArg(), 42).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM user_settings WHERE user_id = \?`).WithArgs(42).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \? WHERE subject_type = \? AND subject_id = \?`).WithArgs(sqlmock.AnyArg(), "user", 42).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	expectStatus(t, testutil.Request(r, "DELETE", "/v1/me", nil, nil), 204)
	if got != 1 {
		t.Fatalf("user.deleted published %d times", got)
	}

	// already deleted: still 204, no second event
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET status = 'deleted'`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	expectStatus(t, testutil.Request(r, "DELETE", "/v1/me", nil, nil), 204)
	if got != 1 {
		t.Fatal("event published for an already deleted user")
	}
}

// ---- admin ----

var adminRowColumns = []string{"id", "display_name", "mobile_last4", "primary_role", "status", "verification_status", "is_minor", "guardian_consent_status", "onboarding_step", "created_at", "last_login_at", "suspended_reason"}

func adminRow(id int64, status string) []driverValue {
	return []driverValue{id, "Asha", "3210", "player", status, "verified", false, "not_required", "done", time.Now(), nil, nil}
}

func TestAdminUsersNeedsPermission(t *testing.T) {
	_, _, r := adminSetup(t, "dashboard.read")
	for _, c := range []struct{ m, p string }{{"GET", "/v1/admin/users"}, {"GET", "/v1/admin/users/1"}, {"POST", "/v1/admin/users/1/suspend"}, {"POST", "/v1/admin/users/1/unsuspend"}} {
		w := testutil.Request(r, c.m, c.p, map[string]any{"reason": "x"}, nil)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s: %d", c.m, c.p, w.Code)
		}
	}
	_, _, r = adminSetup(t, "users.read")
	if w := testutil.Request(r, "POST", "/v1/admin/users/1/suspend", map[string]any{"reason": "x"}, nil); w.Code != 403 {
		t.Fatal("users.read must not suspend", w.Code)
	}
}

func TestAdminListFilters(t *testing.T) {
	d, mock, r := adminSetup(t, "users.read")
	expectStatus(t, testutil.Request(r, "GET", "/v1/admin/users?role=king", nil, nil), 422)
	expectStatus(t, testutil.Request(r, "GET", "/v1/admin/users?isMinor=maybe", nil, nil), 422)
	expectStatus(t, testutil.Request(r, "GET", "/v1/admin/users?q=%23abc", nil, nil), 422)

	// id search
	mock.ExpectQuery(regexp.QuoteMeta(`FROM users u WHERE u.id = ? ORDER BY u.id DESC LIMIT ?`)).WithArgs(42, 21).
		WillReturnRows(sqlmock.NewRows(adminRowColumns).AddRow(adminRow(42, "active")...))
	mock.ExpectQuery(`SELECT user_id, role FROM user_roles WHERE user_id IN`).WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"u", "r"}).AddRow(42, "player"))
	b := expectStatus(t, testutil.Request(r, "GET", "/v1/admin/users?q=%2342", nil, nil), 200)
	var rows []AdminUserRow
	_ = json.Unmarshal(b.Data, &rows)
	if len(rows) != 1 || rows[0].MobileMasked != "******3210" || rows[0].Roles[0] != "player" {
		t.Fatalf("%+v", rows)
	}

	// mobile search goes by hash, never the raw number
	hash := d.Crypto.Hash("mobile", "+919876543210")
	mock.ExpectQuery(regexp.QuoteMeta(`WHERE u.mobile_hash = ? AND u.status = ? ORDER BY`)).WithArgs(hash, "active", 21).
		WillReturnRows(sqlmock.NewRows(adminRowColumns))
	expectStatus(t, testutil.Request(r, "GET", "/v1/admin/users?q=9876543210&status=active", nil, nil), 200)

	// name prefix with LIKE wildcards escaped, minor filter, page of 1 with next cursor
	mock.ExpectQuery(regexp.QuoteMeta(`WHERE u.display_name LIKE ? AND u.is_minor = ? ORDER BY u.id DESC LIMIT ?`)).WithArgs(`as\%`+"%", true, 2).
		WillReturnRows(sqlmock.NewRows(adminRowColumns).AddRow(adminRow(9, "active")...).AddRow(adminRow(8, "active")...))
	mock.ExpectQuery(`SELECT user_id, role FROM user_roles`).WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"u", "r"}))
	b = expectStatus(t, testutil.Request(r, "GET", "/v1/admin/users?q=as%25&isMinor=true&limit=1", nil, nil), 200)
	if b.Meta["nextCursor"] == nil {
		t.Fatal("expected nextCursor", b.Meta)
	}
}

func TestAdminDetailAudited(t *testing.T) {
	d, mock, r := adminSetup(t, "users.read")
	nameEnc, _ := d.Crypto.EncryptString("Asha Kulkarni")
	mock.ExpectQuery(`FROM users u WHERE u.id = \?$`).WithArgs(42).WillReturnRows(sqlmock.NewRows(adminRowColumns).AddRow(adminRow(42, "active")...))
	usertest.ExpectRoles(mock, 42, "player")
	mock.ExpectQuery(`FROM identity_verifications WHERE user_id = \?`).WithArgs(42).
		WillReturnRows(sqlmock.NewRows([]string{"p", "s", "l", "n", "g", "v"}).AddRow("digilocker_direct", "verified", "1234", nameEnc, "F", time.Now()))
	mock.ExpectQuery(`FROM guardian_consents WHERE user_id = \?`).WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"r"}))
	mock.ExpectQuery(`SELECT \(SELECT COUNT\(\*\) FROM events`).WithArgs(42, 42, 42, 42).WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c", "d"}).AddRow(1, 2, 3, 4))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(7, "user.view", "user", "42", nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	b := expectStatus(t, testutil.Request(r, "GET", "/v1/admin/users/42", nil, nil), 200)
	var out AdminUserDetail
	_ = json.Unmarshal(b.Data, &out)
	if out.Verification == nil || *out.Verification.VerifiedName != "Asha Kulkarni" || out.Guardian != nil || out.Counts.Posts != 3 {
		t.Fatalf("%+v", out)
	}

	mock.ExpectQuery(`FROM users u WHERE u.id = \?$`).WithArgs(99).WillReturnRows(sqlmock.NewRows(adminRowColumns))
	expectStatus(t, testutil.Request(r, "GET", "/v1/admin/users/99", nil, nil), 404)
}

func TestAdminSuspendUnsuspend(t *testing.T) {
	_, mock, r := adminSetup(t, "users.write")
	expectStatus(t, testutil.Request(r, "POST", "/v1/admin/users/42/suspend", map[string]any{"reason": "  "}, nil), 422)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM users u WHERE u.id = \? FOR UPDATE`).WithArgs(42).WillReturnRows(sqlmock.NewRows(adminRowColumns).AddRow(adminRow(42, "active")...))
	mock.ExpectExec(`UPDATE users SET status = \?, suspended_reason = \? WHERE id = \?`).WithArgs("suspended", "spam", 42).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at`).WithArgs(sqlmock.AnyArg(), "user", 42).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(7, "user.suspend", "user", "42", nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	usertest.ExpectRoles(mock, 42, "player")
	b := expectStatus(t, testutil.Request(r, "POST", "/v1/admin/users/42/suspend", map[string]any{"reason": "spam"}, nil), 200)
	var row AdminUserRow
	_ = json.Unmarshal(b.Data, &row)
	if row.Status != "suspended" {
		t.Fatal(row.Status)
	}

	// already suspended -> 409
	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WithArgs(42).WillReturnRows(sqlmock.NewRows(adminRowColumns).AddRow(adminRow(42, "suspended")...))
	mock.ExpectRollback()
	expectStatus(t, testutil.Request(r, "POST", "/v1/admin/users/42/suspend", map[string]any{"reason": "spam"}, nil), 409)

	// unsuspend an active user -> 409; deleted user -> 404
	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WithArgs(42).WillReturnRows(sqlmock.NewRows(adminRowColumns).AddRow(adminRow(42, "active")...))
	mock.ExpectRollback()
	expectStatus(t, testutil.Request(r, "POST", "/v1/admin/users/42/unsuspend", map[string]any{}, nil), 409)
	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WithArgs(43).WillReturnRows(sqlmock.NewRows(adminRowColumns).AddRow(adminRow(43, "deleted")...))
	mock.ExpectRollback()
	expectStatus(t, testutil.Request(r, "POST", "/v1/admin/users/43/unsuspend", map[string]any{}, nil), 404)

	// unsuspend happy
	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WithArgs(42).WillReturnRows(sqlmock.NewRows(adminRowColumns).AddRow(adminRow(42, "suspended")...))
	mock.ExpectExec(`UPDATE users SET status = \?, suspended_reason = \?`).WithArgs("active", nil, 42).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(7, "user.unsuspend", "user", "42", nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	usertest.ExpectRoles(mock, 42, "player")
	expectStatus(t, testutil.Request(r, "POST", "/v1/admin/users/42/unsuspend", map[string]any{}, nil), 200)
}

type driverValue = driver.Value

// Only general policies block every user; organiser terms only organisers. Guardian consent and
// the location notice are accepted in their own flows and never appear in pendingPolicies.
func TestPendingPoliciesScopedByRole(t *testing.T) {
	_, mock, r := userSetup(t, nil)
	usertest.ExpectMe(mock, usertest.MeRow{ID: 42, DisplayName: "Asha", PrimaryRole: "organiser", Roles: []string{"organiser"},
		Pending: [][]driverValue{{int64(6), "organiser_terms", 1, "Organiser Terms"}}})
	b := expectStatus(t, testutil.Request(r, "GET", "/v1/me", nil, nil), 200)
	var m Me
	_ = json.Unmarshal(b.Data, &m)
	if len(m.PendingPolicies) != 1 || m.PendingPolicies[0].DocType != "organiser_terms" {
		t.Fatalf("pending = %+v", m.PendingPolicies)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

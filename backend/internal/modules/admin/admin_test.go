package admin

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
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/auth/tokens"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/testutil"
)

func init() { bcryptCost = bcrypt.MinCost }

type body struct {
	Data  json.RawMessage `json:"data"`
	Meta  map[string]any  `json:"meta"`
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func expect(t *testing.T, w *httptest.ResponseRecorder, status int) body {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, want %d: %s", w.Code, status, w.Body.String())
	}
	var b body
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &b)
	}
	return b
}

func setup(t *testing.T, cfg map[string]any, perms ...string) (*app.Deps, sqlmock.Sqlmock, *gin.Engine) {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	r, rt := testutil.AdminEngine(7, perms)
	New(d).Routes(rt)
	return d, mock, r
}

var adminColNames = []string{"id", "email", "name", "status", "last_login_at", "rid", "rname", "perms"}

func adminRow(id int64, status, role, perms string) []driver.Value {
	return []driver.Value{id, "ops@athlix.test", "Ops", status, nil, int64(2), role, perms}
}

var authCols = []string{"id", "password_hash", "totp_secret_enc", "role_id", "status", "failed_logins", "locked_until"}

type account struct {
	hash   string
	secret string
	enc    []byte
}

func newAccount(t *testing.T, d *app.Deps, password string) account {
	t.Helper()
	h, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "Athlix", AccountName: "ops@athlix.test"})
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := d.Crypto.EncryptString(key.Secret())
	return account{hash: string(h), secret: key.Secret(), enc: enc}
}

func (a account) code(t *testing.T) string {
	c, err := totp.GenerateCode(a.secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func login(r *gin.Engine, email, pw, code string) *httptest.ResponseRecorder {
	return testutil.Request(r, "POST", "/v1/admin/auth/login", map[string]any{"email": email, "password": pw, "totpCode": code}, nil)
}

func TestLoginHappyAndReplay(t *testing.T) {
	d, mock, r := setup(t, map[string]any{"admin.access_token_ttl_minutes": 5, "admin.refresh_token_ttl_hours": 2})
	acc := newAccount(t, d, "s3cret-pass")
	code := acc.code(t)

	expect(t, login(r, "", "", ""), 422)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE email = \? FOR UPDATE`).WithArgs("ops@athlix.test").
		WillReturnRows(sqlmock.NewRows(authCols).AddRow(1, acc.hash, acc.enc, 2, "active", 3, nil))
	mock.ExpectExec(`UPDATE admin_users SET failed_logins = 0, locked_until = NULL, last_login_at = \?`).WithArgs(sqlmock.AnyArg(), 1).WillReturnResult(sqlmock.NewResult(0, 1))
	expectEvent(mock, d, 1, "ops@athlix.test", OutcomeSuccess, ReasonOK)
	mock.ExpectExec(`INSERT INTO refresh_tokens`).WithArgs("admin", 1, sqlmock.AnyArg(), sqlmock.AnyArg(), nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`FROM admin_users a JOIN admin_roles r ON r.id = a.role_id WHERE a.id = \?$`).WithArgs(1).
		WillReturnRows(sqlmock.NewRows(adminColNames).AddRow(adminRow(1, "active", "operations", `["users.read","dashboard.read"]`)...))
	b := expect(t, login(r, " OPS@athlix.test ", "s3cret-pass", code), 200)
	var out LoginResult
	_ = json.Unmarshal(b.Data, &out)
	claims, err := d.Tokens.ParseAdmin(out.Tokens.AccessToken)
	if err != nil || claims.SubjectID() != 1 || len(claims.Permissions) != 2 || out.Admin.Role.Name != "operations" {
		t.Fatalf("%+v %v", out, err)
	}
	if ttl := time.Until(out.Tokens.AccessTokenExpiresAt); ttl > 5*time.Minute+time.Second || ttl < 4*time.Minute {
		t.Fatalf("access ttl %v, want admin.access_token_ttl_minutes=5", ttl)
	}
	if ttl := time.Until(out.Tokens.RefreshTokenExpiresAt); ttl > 2*time.Hour+time.Second || ttl < time.Hour+59*time.Minute {
		t.Fatalf("refresh ttl %v, want admin.refresh_token_ttl_hours=2", ttl)
	}
	if _, err := d.Tokens.ParseUser(out.Tokens.AccessToken); err == nil {
		t.Fatal("admin token must not parse as a user token")
	}

	// the same TOTP code cannot be used twice: counted as a failure, generic error
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE email`).WillReturnRows(sqlmock.NewRows(authCols).AddRow(1, acc.hash, acc.enc, 2, "active", 0, nil))
	mock.ExpectExec(`UPDATE admin_users SET failed_logins = \?, locked_until = \? WHERE id = \?`).WithArgs(1, nil, 1).WillReturnResult(sqlmock.NewResult(0, 1))
	expectEvent(mock, d, 1, "ops@athlix.test", OutcomeFailure, ReasonTOTPReplayed)
	mock.ExpectCommit()
	b = expect(t, login(r, "ops@athlix.test", "s3cret-pass", code), 401)
	if b.Error.Code != "UNAUTHENTICATED" {
		t.Fatal(b.Error.Code)
	}
}

func TestLoginFailuresAndLockout(t *testing.T) {
	d, mock, r := setup(t, map[string]any{"admin.max_failed_logins": 3, "admin.lockout_minutes": 10})
	acc := newAccount(t, d, "right")

	// wrong password: counter goes up, same generic error as a wrong code
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE email`).WillReturnRows(sqlmock.NewRows(authCols).AddRow(1, acc.hash, acc.enc, 2, "active", 0, nil))
	mock.ExpectExec(`UPDATE admin_users SET failed_logins = \?, locked_until = \?`).WithArgs(1, nil, 1).WillReturnResult(sqlmock.NewResult(0, 1))
	expectEvent(mock, d, 1, "ops@athlix.test", OutcomeFailure, ReasonBadPassword)
	mock.ExpectCommit()
	pwMsg := expect(t, login(r, "ops@athlix.test", "wrong", acc.code(t)), 401)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE email`).WillReturnRows(sqlmock.NewRows(authCols).AddRow(1, acc.hash, acc.enc, 2, "active", 1, nil))
	mock.ExpectExec(`UPDATE admin_users SET failed_logins = \?, locked_until = \?`).WithArgs(2, nil, 1).WillReturnResult(sqlmock.NewResult(0, 1))
	expectEvent(mock, d, 1, "ops@athlix.test", OutcomeFailure, ReasonBadTOTP)
	mock.ExpectCommit()
	codeMsg := expect(t, login(r, "ops@athlix.test", "right", "000000"), 401)
	if string(pwMsg.Data) != string(codeMsg.Data) || pwMsg.Error.Code != codeMsg.Error.Code {
		t.Fatal("errors must not say which factor failed")
	}

	// third failure reaches admin.max_failed_logins=3: lock for 10 minutes, counter reset
	var lockedUntil time.Time
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE email`).WillReturnRows(sqlmock.NewRows(authCols).AddRow(1, acc.hash, acc.enc, 2, "active", 2, nil))
	mock.ExpectExec(`UPDATE admin_users SET failed_logins = \?, locked_until = \?`).WithArgs(0, timeCapture{&lockedUntil}, 1).WillReturnResult(sqlmock.NewResult(0, 1))
	expectEvent(mock, d, 1, "ops@athlix.test", OutcomeFailure, ReasonBadPassword)
	mock.ExpectCommit()
	expect(t, login(r, "ops@athlix.test", "wrong", "000000"), 401)
	if m := time.Until(lockedUntil).Minutes(); m < 9.9 || m > 10.1 {
		t.Fatalf("locked for %.1f minutes, want admin.lockout_minutes=10", m)
	}

	// locked: even the right password + code fail, nothing is updated
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE email`).WillReturnRows(sqlmock.NewRows(authCols).AddRow(1, acc.hash, acc.enc, 2, "active", 0, time.Now().Add(5*time.Minute)))
	expectEvent(mock, d, 1, "ops@athlix.test", OutcomeFailure, ReasonLocked)
	mock.ExpectCommit()
	expect(t, login(r, "ops@athlix.test", "right", acc.code(t)), 401)

	// disabled and unknown accounts: generic 401
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE email`).WillReturnRows(sqlmock.NewRows(authCols).AddRow(1, acc.hash, acc.enc, 2, "disabled", 0, nil))
	expectEvent(mock, d, 1, "ops@athlix.test", OutcomeFailure, ReasonDisabled)
	mock.ExpectCommit()
	expect(t, login(r, "ops@athlix.test", "right", acc.code(t)), 401)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE email`).WillReturnRows(sqlmock.NewRows(authCols))
	expectEvent(mock, d, nil, "nobody@athlix.test", OutcomeFailure, ReasonUnknownEmail)
	mock.ExpectCommit()
	expect(t, login(r, "nobody@athlix.test", "x", "123456"), 401)
}

// expectEvent expects one admin_login_events row. The email is only ever stored as its
// HMAC, and no argument may carry the password or the TOTP code.
func expectEvent(mock sqlmock.Sqlmock, d *app.Deps, adminID any, email, outcome, reason string) {
	mock.ExpectExec(`INSERT INTO admin_login_events \(admin_id, email_hash, ip, user_agent, outcome, reason\)`).
		WithArgs(adminID, d.Crypto.Hash(emailHashPurpose, email), sqlmock.AnyArg(), sqlmock.AnyArg(), outcome, reason).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

type timeCapture struct{ dst *time.Time }

func (c timeCapture) Match(v driver.Value) bool {
	t, ok := v.(time.Time)
	if ok {
		*c.dst = t
	}
	return ok
}

func TestLoginRateLimitPerIP(t *testing.T) {
	d, mock, r := setup(t, map[string]any{"admin.login.max_per_ip": 3, "admin.login.window_minutes": 1})
	for i := 0; i < 3; i++ {
		mock.ExpectBegin()
		mock.ExpectQuery(`FROM admin_users WHERE email`).WillReturnRows(sqlmock.NewRows(authCols))
		expectEvent(mock, d, nil, "nobody@athlix.test", OutcomeFailure, ReasonUnknownEmail)
		mock.ExpectCommit()
		expect(t, login(r, "nobody@athlix.test", "x", "123456"), 401)
	}
	b := expect(t, login(r, "nobody@athlix.test", "x", "123456"), 429)
	if b.Error.Code != "RATE_LIMITED" {
		t.Fatal(b.Error.Code)
	}
}

var tokenCols = []string{"id", "subject_id", "family_id", "device_id", "expires_at", "revoked_at"}

func TestRefreshUsesCurrentRoleAndFixedExpiry(t *testing.T) {
	d, mock, r := setup(t, nil)
	sessionEnd := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Millisecond)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM refresh_tokens WHERE token_hash = \? AND subject_type = \?`).WithArgs(crypto.SHA256("good"), "admin").
		WillReturnRows(sqlmock.NewRows(tokenCols).AddRow(5, 1, "fam", nil, sessionEnd, nil))
	mock.ExpectQuery(`SELECT status FROM admin_users WHERE id = \?`).WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"s"}).AddRow("active"))
	mock.ExpectExec(`INSERT INTO refresh_tokens`).WithArgs("admin", 1, sqlmock.AnyArg(), "fam", nil, sessionEnd).WillReturnResult(sqlmock.NewResult(6, 1))
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \?, replaced_by = \?`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`WHERE a.id = \?`).WithArgs(1).WillReturnRows(sqlmock.NewRows(adminColNames).AddRow(adminRow(1, "active", "moderator", `["reports.read"]`)...))
	b := expect(t, testutil.Request(r, "POST", "/v1/admin/auth/refresh", map[string]any{"refreshToken": "good"}, nil), 200)
	var p tokens.Pair
	_ = json.Unmarshal(b.Data, &p)
	c, err := d.Tokens.ParseAdmin(p.AccessToken)
	if err != nil || len(c.Permissions) != 1 || c.Permissions[0] != "reports.read" {
		t.Fatal("refresh must copy the role's current permissions", err)
	}

	// disabled admin cannot refresh
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM refresh_tokens`).WillReturnRows(sqlmock.NewRows(tokenCols).AddRow(5, 1, "fam", nil, sessionEnd, nil))
	mock.ExpectQuery(`SELECT status FROM admin_users`).WillReturnRows(sqlmock.NewRows([]string{"s"}).AddRow("disabled"))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/admin/auth/refresh", map[string]any{"refreshToken": "x"}, nil), 401)
}

func TestLogoutAndMe(t *testing.T) {
	_, mock, r := setup(t, nil)
	mock.ExpectExec(`UPDATE refresh_tokens t JOIN`).WithArgs(crypto.SHA256("tok"), "admin", 7, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	expect(t, testutil.Request(r, "POST", "/v1/admin/auth/logout", map[string]any{"refreshToken": "tok"}, nil), 204)
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \? WHERE subject_type = \? AND subject_id = \?`).WithArgs(sqlmock.AnyArg(), "admin", 7).WillReturnResult(sqlmock.NewResult(0, 1))
	expect(t, testutil.Request(r, "POST", "/v1/admin/auth/logout", nil, nil), 204)

	mock.ExpectQuery(`WHERE a.id = \?`).WithArgs(7).WillReturnRows(sqlmock.NewRows(adminColNames).AddRow(adminRow(7, "active", "operations", `["users.read"]`)...))
	b := expect(t, testutil.Request(r, "GET", "/v1/admin/me", nil, nil), 200)
	var a AdminUser
	_ = json.Unmarshal(b.Data, &a)
	if a.ID != 7 || a.Role.ID != 2 || a.Permissions[0] != "users.read" || a.LastLoginAt != nil {
		t.Fatalf("%+v", a)
	}
}

func TestPermissionsEnforced(t *testing.T) {
	_, _, r := setup(t, nil, "users.read")
	for _, c := range []struct{ m, p string }{
		{"GET", "/v1/admin/dashboard"}, {"GET", "/v1/admin/admins"}, {"POST", "/v1/admin/admins"},
		{"PUT", "/v1/admin/admins/1"}, {"POST", "/v1/admin/admins/1/reset-credentials"},
		{"GET", "/v1/admin/roles"}, {"POST", "/v1/admin/roles"}, {"PUT", "/v1/admin/roles/1"},
		{"GET", "/v1/admin/audit-log"}, {"GET", "/v1/admin/login-events"},
	} {
		if w := testutil.Request(r, c.m, c.p, map[string]any{}, nil); w.Code != 403 {
			t.Errorf("%s %s: %d", c.m, c.p, w.Code)
		}
	}
}

func TestDashboard(t *testing.T) {
	_, mock, r := setup(t, nil, "dashboard.read")
	mock.ExpectQuery(`SELECT\s+\(SELECT COUNT\(\*\) FROM users WHERE status <> 'deleted'\)`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(strings.Split("a b c d e f g h i j k l m", " ")).AddRow(10, 8, 2, 3, 4, 1, 2, 5, 6, 150000, 2000, 7, 9))
	b := expect(t, testutil.Request(r, "GET", "/v1/admin/dashboard", nil, nil), 200)
	var d Dashboard
	_ = json.Unmarshal(b.Data, &d)
	if d.Users.Total != 10 || d.Users.Minors != 2 || d.Events.Upcoming != 2 || d.Organisers.PendingReview != 5 ||
		d.Revenue.ListingPaidPaiseLast30Days != 150000 || d.Revenue.ActiveSubscriptions != 7 || d.Communities.Active != 9 {
		t.Fatalf("%+v", d)
	}
}

func TestCreateAdmin(t *testing.T) {
	d, mock, r := setup(t, nil, "admins.manage")
	b := expect(t, testutil.Request(r, "POST", "/v1/admin/admins", map[string]any{"email": "bad", "name": "", "roleId": 0}, nil), 422)
	if len(b.Error.Fields) != 3 {
		t.Fatal(b.Error.Fields)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, name, description, permissions FROM admin_roles WHERE id = \?$`).WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "d", "p"}).AddRow(2, "operations", nil, `["users.read"]`))
	var totpEnc []byte
	mock.ExpectExec(`INSERT INTO admin_users \(email, name, password_hash, totp_secret_enc, key_version, role_id\)`).
		WithArgs("new@athlix.test", "New Admin", sqlmock.AnyArg(), bytesCapture{&totpEnc}, d.Crypto.KeyVersion(), 2).WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(7, "admin.create", "admin", "12", `{"roleId":2}`, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	b = expect(t, testutil.Request(r, "POST", "/v1/admin/admins", map[string]any{"email": "New@Athlix.test", "name": " New Admin ", "roleId": 2}, nil), 201)
	var out CreatedAdmin
	_ = json.Unmarshal(b.Data, &out)
	if out.Admin.ID != 12 || len(out.TemporaryPassword) < 20 || !strings.HasPrefix(out.TotpURI, "otpauth://totp/Athlix:") {
		t.Fatalf("%+v", out)
	}
	secret, err := d.Crypto.DecryptString(totpEnc)
	if err != nil || !strings.Contains(out.TotpURI, "secret="+secret) {
		t.Fatal("stored TOTP secret must be encrypted and match the URI", err)
	}

	// duplicate email
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_roles WHERE id`).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "d", "p"}).AddRow(2, "operations", nil, `[]`))
	mock.ExpectExec(`INSERT INTO admin_users`).WillReturnError(errDup)
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/admin/admins", map[string]any{"email": "new@athlix.test", "name": "X", "roleId": 2}, nil), 409)

	// unknown role
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_roles WHERE id`).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "d", "p"}))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/admin/admins", map[string]any{"email": "x@athlix.test", "name": "X", "roleId": 99}, nil), 422)
}

type bytesCapture struct{ dst *[]byte }

func (c bytesCapture) Match(v driver.Value) bool {
	b, ok := v.([]byte)
	*c.dst = b
	return ok
}

type dupErr struct{}

func (dupErr) Error() string { return "Error 1062: Duplicate entry 'x' for key 'uq_admin_email'" }

var errDup = dupErr{}

func TestUpdateAdmin(t *testing.T) {
	_, mock, r := setup(t, nil, "admins.manage")
	// cannot disable yourself (caller is 7)
	b := expect(t, testutil.Request(r, "PUT", "/v1/admin/admins/7", map[string]any{"status": "disabled"}, nil), 422)
	if b.Error.Fields["status"] == "" {
		t.Fatal(b.Error.Fields)
	}
	expect(t, testutil.Request(r, "PUT", "/v1/admin/admins/3", map[string]any{"status": "gone"}, nil), 422)

	// unknown admin
	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE a.id = \? FOR UPDATE`).WithArgs(99).WillReturnRows(sqlmock.NewRows(adminColNames))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/admins/99", map[string]any{"name": "X"}, nil), 404)

	// the last active super admin cannot be disabled
	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE a.id = \? FOR UPDATE`).WithArgs(3).WillReturnRows(sqlmock.NewRows(adminColNames).AddRow(adminRow(3, "active", "super_admin", `["*"]`)...))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM admin_users a JOIN admin_roles r`).WithArgs(3).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/admins/3", map[string]any{"status": "disabled"}, nil), 409)

	// disable another admin: sessions revoked, audited with flags only
	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE a.id = \? FOR UPDATE`).WithArgs(3).WillReturnRows(sqlmock.NewRows(adminColNames).AddRow(adminRow(3, "active", "operations", `["users.read"]`)...))
	mock.ExpectQuery(`FROM admin_roles WHERE id = \?$`).WithArgs(4).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "d", "p"}).AddRow(4, "finance", nil, `["payments.read"]`))
	mock.ExpectExec(`UPDATE admin_users SET name = \?, role_id = \?, status = \? WHERE id = \?`).WithArgs("Renamed", 4, "disabled", 3).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at`).WithArgs(sqlmock.AnyArg(), "admin", 3).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(7, "admin.update", "admin", "3", `{"nameChanged":true,"roleId":4,"status":"disabled"}`, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	b = expect(t, testutil.Request(r, "PUT", "/v1/admin/admins/3", map[string]any{"name": "Renamed", "roleId": 4, "status": "disabled"}, nil), 200)
	var a AdminUser
	_ = json.Unmarshal(b.Data, &a)
	if a.Status != "disabled" || a.Role.Name != "finance" || a.Name != "Renamed" {
		t.Fatalf("%+v", a)
	}

	// no change: nothing written
	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE a.id = \? FOR UPDATE`).WithArgs(3).WillReturnRows(sqlmock.NewRows(adminColNames).AddRow(adminRow(3, "active", "operations", `[]`)...))
	mock.ExpectCommit()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/admins/3", map[string]any{"name": "Ops"}, nil), 200)
}

func TestResetCredentials(t *testing.T) {
	_, mock, r := setup(t, nil, "admins.manage")
	mock.ExpectBegin()
	mock.ExpectQuery(`WHERE a.id = \? FOR UPDATE`).WithArgs(3).WillReturnRows(sqlmock.NewRows(adminColNames).AddRow(adminRow(3, "active", "operations", `[]`)...))
	mock.ExpectExec(`UPDATE admin_users SET password_hash = \?, totp_secret_enc = \?, key_version = \?, failed_logins = 0, locked_until = NULL WHERE id = \?`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 3).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at`).WithArgs(sqlmock.AnyArg(), "admin", 3).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(7, "admin.reset_credentials", "admin", "3", nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	b := expect(t, testutil.Request(r, "POST", "/v1/admin/admins/3/reset-credentials", nil, nil), 200)
	var c Credentials
	_ = json.Unmarshal(b.Data, &c)
	if c.TemporaryPassword == "" || !strings.HasPrefix(c.TotpURI, "otpauth://") {
		t.Fatalf("%+v", c)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WithArgs(99).WillReturnRows(sqlmock.NewRows(adminColNames))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/admin/admins/99/reset-credentials", nil, nil), 404)
}

func TestRoles(t *testing.T) {
	_, mock, r := setup(t, nil, "admins.manage")
	b := expect(t, testutil.Request(r, "POST", "/v1/admin/roles", map[string]any{"name": "Bad Name", "permissions": []string{"users.read", "nuke.all"}}, nil), 422)
	if b.Error.Fields["name"] == "" || b.Error.Fields["permissions"] == "" {
		t.Fatal(b.Error.Fields)
	}
	expect(t, testutil.Request(r, "POST", "/v1/admin/roles", map[string]any{"name": "everything", "permissions": []string{"*"}}, nil), 422)
	expect(t, testutil.Request(r, "POST", "/v1/admin/roles", map[string]any{"name": "empty", "permissions": []string{}}, nil), 422)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO admin_roles \(name, description, permissions\)`).WithArgs("geo_team", "Districts", `["geo.write","location.review"]`).WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(7, "role.create", "admin_role", "9", nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	b = expect(t, testutil.Request(r, "POST", "/v1/admin/roles", map[string]any{"name": "geo_team", "description": "Districts", "permissions": []string{"location.review", "geo.write", "geo.write"}}, nil), 201)
	var ro Role
	_ = json.Unmarshal(b.Data, &ro)
	if ro.ID != 9 || len(ro.Permissions) != 2 {
		t.Fatalf("%+v", ro)
	}

	// duplicate name
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO admin_roles`).WillReturnError(errDup)
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/admin/roles", map[string]any{"name": "geo_team", "permissions": []string{"geo.write"}}, nil), 409)

	// super_admin is read-only
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_roles WHERE id = \? FOR UPDATE`).WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "d", "p"}).AddRow(1, "super_admin", nil, `["*"]`))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/roles/1", map[string]any{"permissions": []string{"users.read"}}, nil), 409)

	// update permissions, description untouched
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_roles WHERE id = \? FOR UPDATE`).WithArgs(4).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "d", "p"}).AddRow(4, "finance", "Money", `["payments.read"]`))
	mock.ExpectExec(`UPDATE admin_roles SET description = \?, permissions = \? WHERE id = \?`).WithArgs("Money", `["payments.read","payments.refund"]`, 4).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(7, "role.update", "admin_role", "4", `{"permissions":["payments.read","payments.refund"],"roleId":4}`, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/roles/4", map[string]any{"permissions": []string{"payments.refund", "payments.read"}}, nil), 200)

	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE`).WithArgs(77).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "d", "p"}))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/roles/77", map[string]any{"description": "x"}, nil), 404)

	mock.ExpectQuery(`SELECT id, name, description, permissions FROM admin_roles ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "d", "p"}).AddRow(1, "super_admin", nil, `["*"]`))
	expect(t, testutil.Request(r, "GET", "/v1/admin/roles", nil, nil), 200)
}

func TestAuditLog(t *testing.T) {
	_, mock, r := setup(t, nil, "audit.read")
	expect(t, testutil.Request(r, "GET", "/v1/admin/audit-log?adminId=abc", nil, nil), 422)

	cols := []string{"id", "admin_id", "name", "action", "tt", "tid", "details", "created_at"}
	mock.ExpectQuery(regexp.QuoteMeta(`WHERE l.admin_id = ? AND l.action = ? AND l.target_type = ? AND l.target_id = ? ORDER BY l.id DESC LIMIT ?`)).
		WithArgs(3, "user.suspend", "user", "42", 2).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(20, 3, "Ops", "user.suspend", "user", "42", nil, time.Now()).AddRow(19, 3, "Ops", "user.suspend", "user", "42", `{"a":1}`, time.Now()))
	b := expect(t, testutil.Request(r, "GET", "/v1/admin/audit-log?adminId=3&action=user.suspend&targetType=user&targetId=42&limit=1", nil, nil), 200)
	var rows []AuditEntry
	_ = json.Unmarshal(b.Data, &rows)
	if len(rows) != 1 || rows[0].Admin.Name != "Ops" || string(rows[0].Details) != "null" || b.Meta["nextCursor"] == nil {
		t.Fatalf("%+v %v", rows, b.Meta)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`WHERE l.id < ? ORDER BY l.id DESC LIMIT ?`)).WithArgs(20, 2).WillReturnRows(sqlmock.NewRows(cols))
	expect(t, testutil.Request(r, "GET", "/v1/admin/audit-log?limit=1&cursor="+b.Meta["nextCursor"].(string), nil, nil), 200)
}

func TestCreateAdminForCLI(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	if _, _, err := CreateAdmin(context.Background(), d, "nope", "X", "super_admin"); err == nil {
		t.Fatal("expected validation error")
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_roles WHERE name = \?`).WithArgs("super_admin").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "d", "p"}).AddRow(1, "super_admin", nil, `["*"]`))
	mock.ExpectExec(`INSERT INTO admin_users`).WithArgs("root@athlix.test", "Root", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 1).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	a, c, err := CreateAdmin(context.Background(), d, "root@athlix.test", "Root", "super_admin")
	if err != nil || a.ID != 1 || c.TemporaryPassword == "" || c.TotpURI == "" {
		t.Fatal(a, err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM admin_users WHERE email = \?`).WithArgs("root@athlix.test").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`UPDATE admin_users SET password_hash`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at`).WithArgs(sqlmock.AnyArg(), "admin", 1).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if c, err := ResetCredentialsByEmail(context.Background(), d, "ROOT@athlix.test"); err != nil || c.TemporaryPassword == "" {
		t.Fatal(err)
	}
}

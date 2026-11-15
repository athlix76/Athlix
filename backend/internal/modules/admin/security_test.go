package admin

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"athlix/backend/internal/testutil"
)

var authByIDCols = []string{"id", "email", "password_hash", "totp_secret_enc", "role_id", "status", "failed_logins", "locked_until"}

type stringCapture struct{ dst *string }

func (c stringCapture) Match(v driver.Value) bool {
	s, ok := v.(string)
	*c.dst = s
	return ok
}

func changePw(_ any, cur, next, code string) map[string]any {
	return map[string]any{"currentPassword": cur, "newPassword": next, "totpCode": code}
}

func TestChangePassword(t *testing.T) {
	d, mock, r := setup(t, map[string]any{"admin.password_min_length": 10, "admin.max_failed_logins": 3, "admin.refresh_token_ttl_hours": 2})
	acc := newAccount(t, d, "old-password-1")
	const path = "/v1/admin/me/password"

	// missing fields, then policy failures that need no DB (and burn no code)
	b := expect(t, testutil.Request(r, "POST", path, map[string]any{}, nil), 422)
	if len(b.Error.Fields) != 3 {
		t.Fatal(b.Error.Fields)
	}
	b = expect(t, testutil.Request(r, "POST", path, changePw(r, "old-password-1", "short", "123456"), nil), 422)
	if !strings.Contains(b.Error.Fields["newPassword"], "10 to 72") {
		t.Fatalf("min length must come from admin.password_min_length: %v", b.Error.Fields)
	}
	expect(t, testutil.Request(r, "POST", path, changePw(r, "old-password-1", "old-password-1", "123456"), nil), 422)
	expect(t, testutil.Request(r, "POST", path, changePw(r, "old-password-1", strings.Repeat("x", 73), "123456"), nil), 422)

	// wrong current password: counted towards the lockout, 422 (client keeps its session)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE id = \? FOR UPDATE`).WithArgs(7).
		WillReturnRows(sqlmock.NewRows(authByIDCols).AddRow(7, "ops@athlix.test", acc.hash, acc.enc, 2, "active", 0, nil))
	mock.ExpectExec(`UPDATE admin_users SET failed_logins = \?, locked_until = \? WHERE id = \?`).WithArgs(1, nil, 7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	b = expect(t, testutil.Request(r, "POST", path, changePw(r, "wrong-password", "brand-new-secret", acc.code(t)), nil), 422)
	if b.Error.Fields["currentPassword"] == "" {
		t.Fatal(b.Error.Fields)
	}

	// third wrong attempt locks the account
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows(authByIDCols).AddRow(7, "ops@athlix.test", acc.hash, acc.enc, 2, "active", 2, nil))
	mock.ExpectExec(`UPDATE admin_users SET failed_logins = \?, locked_until = \?`).WithArgs(0, sqlmock.AnyArg(), 7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expect(t, testutil.Request(r, "POST", path, changePw(r, "old-password-1", "brand-new-secret", "000000"), nil), 422)

	// locked account: refused, nothing written
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows(authByIDCols).AddRow(7, "ops@athlix.test", acc.hash, acc.enc, 2, "active", 0, time.Now().Add(time.Minute)))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", path, changePw(r, "old-password-1", "brand-new-secret", acc.code(t)), nil), 401)

	// happy path: new hash, every session revoked, new session issued, audited
	code := acc.code(t)
	var stored string
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows(authByIDCols).AddRow(7, "ops@athlix.test", acc.hash, acc.enc, 2, "active", 1, nil))
	mock.ExpectExec(`UPDATE admin_users SET password_hash = \?, failed_logins = 0, locked_until = NULL WHERE id = \?`).
		WithArgs(stringCapture{&stored}, 7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \? WHERE subject_type = \? AND subject_id = \? AND revoked_at IS NULL`).
		WithArgs(sqlmock.AnyArg(), "admin", 7).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`INSERT INTO refresh_tokens`).WithArgs("admin", 7, sqlmock.AnyArg(), sqlmock.AnyArg(), nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(7, "admin.change_password", "admin", "7", nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`WHERE a.id = \?$`).WithArgs(7).WillReturnRows(sqlmock.NewRows(adminColNames).AddRow(adminRow(7, "active", "operations", `["users.read"]`)...))
	b = expect(t, testutil.Request(r, "POST", path, changePw(r, "old-password-1", "brand-new-secret", code), nil), 200)
	var out ChangePasswordResult
	_ = json.Unmarshal(b.Data, &out)
	if c, err := d.Tokens.ParseAdmin(out.Tokens.AccessToken); err != nil || c.SubjectID() != 7 || out.Tokens.RefreshToken == "" {
		t.Fatalf("%+v %v", out, err)
	}
	if bcrypt.CompareHashAndPassword([]byte(stored), []byte("brand-new-secret")) != nil {
		t.Fatal("stored value must be the bcrypt hash of the new password")
	}

	// the same code cannot be used again (replay)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows(authByIDCols).AddRow(7, "ops@athlix.test", acc.hash, acc.enc, 2, "active", 0, nil))
	mock.ExpectExec(`UPDATE admin_users SET failed_logins = \?`).WithArgs(1, nil, 7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expect(t, testutil.Request(r, "POST", path, changePw(r, "old-password-1", "another-new-secret", code), nil), 422)

	// the new password must not contain the email name (next step's code: skew allows it)
	nextCode, err := totp.GenerateCode(acc.secret, time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows(authByIDCols).AddRow(7, "ops@athlix.test", acc.hash, acc.enc, 2, "active", 0, nil))
	mock.ExpectCommit()
	b = expect(t, testutil.Request(r, "POST", path, changePw(r, "old-password-1", "my-OPS-password", nextCode), nil), 422)
	if !strings.Contains(b.Error.Fields["newPassword"], "email") {
		t.Fatal(b.Error.Fields)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLoginEventRecordsClientAndTruncatesUA(t *testing.T) {
	d, mock, r := setup(t, nil)
	ua := strings.Repeat("é", 300)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_users WHERE email`).WillReturnRows(sqlmock.NewRows(authCols))
	mock.ExpectExec(`INSERT INTO admin_login_events`).
		WithArgs(nil, d.Crypto.Hash(emailHashPurpose, "who@athlix.test"), "192.0.2.1", strings.Repeat("é", 255), OutcomeFailure, ReasonUnknownEmail).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	w := testutil.Request(r, "POST", "/v1/admin/auth/login", map[string]any{"email": "Who@athlix.test", "password": "pw-secret", "totpCode": "123456"}, map[string]string{"User-Agent": ua})
	expect(t, w, 401)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLoginEventsListing(t *testing.T) {
	d, mock, r := setup(t, nil, "audit.read")
	expect(t, testutil.Request(r, "GET", "/v1/admin/login-events?outcome=maybe", nil, nil), 422)
	expect(t, testutil.Request(r, "GET", "/v1/admin/login-events?adminId=x", nil, nil), 422)

	cols := []string{"id", "admin_id", "name", "outcome", "reason", "ip", "ua", "created_at"}
	mock.ExpectQuery(regexp.QuoteMeta(`WHERE e.admin_id = ? AND e.email_hash = ? AND e.outcome = ? ORDER BY e.id DESC LIMIT ?`)).
		WithArgs(3, d.Crypto.Hash(emailHashPurpose, "ops@athlix.test"), "failure", 2).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(30, 3, "Ops", "failure", "bad_totp", "10.0.0.1", "Firefox", time.Now()).
			AddRow(29, 3, "Ops", "failure", "bad_password", nil, nil, time.Now()))
	b := expect(t, testutil.Request(r, "GET", "/v1/admin/login-events?adminId=3&email=%20OPS@athlix.test&outcome=failure&limit=1", nil, nil), 200)
	var rows []LoginEvent
	_ = json.Unmarshal(b.Data, &rows)
	if len(rows) != 1 || rows[0].Admin == nil || rows[0].Admin.Name != "Ops" || *rows[0].IP != "10.0.0.1" || b.Meta["nextCursor"] == nil {
		t.Fatalf("%+v %v", rows, b.Meta)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`WHERE e.id < ? ORDER BY e.id DESC LIMIT ?`)).WithArgs(30, 2).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(28, nil, "", "failure", "unknown_email", nil, nil, time.Now()))
	b = expect(t, testutil.Request(r, "GET", "/v1/admin/login-events?limit=1&cursor="+b.Meta["nextCursor"].(string), nil, nil), 200)
	rows = nil
	_ = json.Unmarshal(b.Data, &rows)
	if len(rows) != 1 || rows[0].Admin != nil {
		t.Fatalf("unknown email has no admin: %+v", rows)
	}
}

func TestPurgeLoginEventsJob(t *testing.T) {
	d, mock := testutil.Deps(t, map[string]any{"admin.login_events_retention_days": 30})
	job := New(d).Jobs()["admin.purge_login_events"]
	if job == nil {
		t.Fatal("job not registered")
	}
	var cutoff time.Time
	mock.ExpectExec(`DELETE FROM admin_login_events WHERE created_at < \? ORDER BY id LIMIT \?`).WithArgs(timeCapture{&cutoff}, 500).WillReturnResult(sqlmock.NewResult(0, 500))
	mock.ExpectExec(`DELETE FROM admin_login_events`).WithArgs(sqlmock.AnyArg(), 500).WillReturnResult(sqlmock.NewResult(0, 12))
	if err := job(context.Background()); err != nil {
		t.Fatal(err)
	}
	if days := time.Since(cutoff).Hours() / 24; days < 29.9 || days > 30.1 {
		t.Fatalf("cutoff %.1f days ago, want admin.login_events_retention_days=30", days)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/app"
	"athlix/backend/internal/testutil"
)

func bootWith(d *app.Deps) bootFunc {
	return func(context.Context) (*app.Deps, func(), error) { return d, func() {}, nil }
}

func runCLI(t *testing.T, boot bootFunc, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut, boot)
	return code, out.String(), errOut.String()
}

func TestUsageErrors(t *testing.T) {
	failBoot := func(context.Context) (*app.Deps, func(), error) {
		t.Fatal("bootstrap must not run on a usage error")
		return nil, nil, nil
	}
	cases := [][]string{
		{},
		{"nope"},
		{"create-admin", "-email", "a@b.test"},
		{"create-admin", "-name", "X"},
		{"reset-credentials"},
		{"import-districts"},
		{"create-admin", "-bogus"},
	}
	for _, args := range cases {
		if code, _, _ := runCLI(t, failBoot, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
	if code, out, _ := runCLI(t, failBoot, "help"); code != 0 || !strings.Contains(out, "import-districts") {
		t.Fatalf("help: %d %q", code, out)
	}
}

func TestBootstrapError(t *testing.T) {
	boot := func(context.Context) (*app.Deps, func(), error) { return nil, nil, errors.New("db down") }
	if code, _, errOut := runCLI(t, boot, "reset-credentials", "-email", "a@b.test"); code != 1 || !strings.Contains(errOut, "bootstrap") {
		t.Fatalf("%d %q", code, errOut)
	}
}

func TestCreateAdminDefaultsToSuperAdmin(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_roles WHERE name = \?`).WithArgs("super_admin").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "d", "p"}).AddRow(1, "super_admin", nil, `["*"]`))
	mock.ExpectExec(`INSERT INTO admin_users`).WithArgs("root@athlix.test", "Root", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(5, 1))
	mock.ExpectCommit()
	code, out, errOut := runCLI(t, bootWith(d), "create-admin", "-email", "root@athlix.test", "-name", "Root")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "Created admin 5") || !strings.Contains(out, "temporary password:") || !strings.Contains(out, "otpauth://totp/") {
		t.Fatalf("stdout %q", out)
	}
	if strings.Contains(errOut, "otpauth") || strings.Contains(errOut, "password:") {
		t.Fatal("credentials must only go to stdout")
	}
}

func TestCreateAdminValidationAndUnknownRole(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	code, _, errOut := runCLI(t, bootWith(d), "create-admin", "-email", "not-an-email", "-name", "Root")
	if code != 1 || !strings.Contains(errOut, "VALIDATION_FAILED") || !strings.Contains(errOut, "email") {
		t.Fatalf("%d %q", code, errOut)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM admin_roles WHERE name = \?`).WithArgs("ghost").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "d", "p"}))
	mock.ExpectRollback()
	if code, _, _ := runCLI(t, bootWith(d), "create-admin", "-email", "a@athlix.test", "-name", "A", "-role", "ghost"); code != 1 {
		t.Fatalf("unknown role: exit %d", code)
	}
}

func TestResetCredentials(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM admin_users WHERE email = \?`).WithArgs("root@athlix.test").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`UPDATE admin_users SET password_hash`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at`).WithArgs(sqlmock.AnyArg(), "admin", 1).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	code, out, errOut := runCLI(t, bootWith(d), "reset-credentials", "-email", "Root@athlix.test")
	if code != 0 || !strings.Contains(out, "otpauth://totp/") {
		t.Fatalf("%d %q %q", code, out, errOut)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM admin_users WHERE email = \?`).WithArgs("ghost@athlix.test").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	if code, _, errOut := runCLI(t, bootWith(d), "reset-credentials", "-email", "ghost@athlix.test"); code != 1 || !strings.Contains(errOut, "NOT_FOUND") {
		t.Fatalf("%d %q", code, errOut)
	}
}

func TestImportDistricts(t *testing.T) {
	if code, _, _ := runCLI(t, nil, "import-districts", "-file", "testdata/missing.csv"); code != 1 {
		t.Fatalf("missing file: exit %d", code)
	}
	d, mock := testutil.Deps(t, nil)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, code FROM states`).WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(23, "TN"))
	mock.ExpectQuery(`SELECT id FROM districts WHERE lgd_code = \?`).WithArgs(640).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT id FROM districts WHERE state_id = \? AND name = \?`).WithArgs(23, "Mayiladuthurai").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`INSERT INTO districts`).WithArgs(23, "Mayiladuthurai", 640, `["Mayavaram"]`).WillReturnResult(sqlmock.NewResult(700, 1))
	mock.ExpectCommit()
	code, out, errOut := runCLI(t, bootWith(d), "import-districts", "-file", "testdata/districts.csv")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{`"inserted": 1`, `"skipped": 1`, `"line": 3`, "unknown state code"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %s: %s", want, out)
		}
	}

	d2, mock2 := testutil.Deps(t, nil)
	mock2.ExpectBegin()
	mock2.ExpectQuery(`SELECT id, code FROM states`).WillReturnError(errors.New("db gone"))
	mock2.ExpectRollback()
	if code, _, _ := runCLI(t, bootWith(d2), "import-districts", "-file", "testdata/districts.csv"); code != 1 {
		t.Fatalf("db error: exit %d", code)
	}
}

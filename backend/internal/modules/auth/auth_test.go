package auth

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/auth/tokens"
	"athlix/backend/internal/modules/user/usertest"
	"athlix/backend/internal/platform/config"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/testutil"
)

type body struct {
	Data  json.RawMessage `json:"data"`
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

func setup(t *testing.T, cfg map[string]any) (*app.Deps, sqlmock.Sqlmock, *gin.Engine) {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	r, rt := testutil.UserEngine(testutil.VerifiedUser(42))
	New(d).Routes(rt)
	return d, mock, r
}

func sent(d *app.Deps) []testutil.SentSMS { return d.SMS.(*testutil.RecordingSMS).Sent }

func requestOTP(t *testing.T, r *gin.Engine, mobile string) OTPTicket {
	t.Helper()
	b := expect(t, testutil.Request(r, "POST", "/v1/auth/otp/request", map[string]any{"mobile": mobile}, nil), 200)
	var tk OTPTicket
	_ = json.Unmarshal(b.Data, &tk)
	return tk
}

func TestRequestOTP(t *testing.T) {
	d, _, r := setup(t, map[string]any{"auth.otp.length": 4, "auth.otp.ttl_seconds": 120, "auth.otp.resend_cooldown_seconds": 45})
	b := expect(t, testutil.Request(r, "POST", "/v1/auth/otp/request", map[string]any{"mobile": "12345"}, nil), 422)
	if b.Error.Fields["mobile"] == "" {
		t.Fatal(b.Error)
	}
	tk := requestOTP(t, r, "+91 98765 43210")
	if tk.RequestID == "" || tk.ExpiresInSeconds != 120 || tk.ResendAfterSeconds != 45 {
		t.Fatalf("%+v", tk)
	}
	msgs := sent(d)
	if len(msgs) != 1 || msgs[0].Template != "login_otp" || msgs[0].Mobile != "919876543210" || len(msgs[0].OTP) != 4 {
		t.Fatalf("%+v", msgs)
	}
	// the Redis challenge holds no raw mobile and no raw code
	vals, _ := d.Redis.HGetAll(context.Background(), "otp:req:"+tk.RequestID).Result()
	for _, v := range vals {
		if strings.Contains(v, "9876543210") || v == msgs[0].OTP {
			t.Fatalf("challenge leaks a secret: %v", vals)
		}
	}
	// resend cooldown
	b = expect(t, testutil.Request(r, "POST", "/v1/auth/otp/request", map[string]any{"mobile": "9876543210"}, nil), 429)
	if b.Error.Code != "RATE_LIMITED" {
		t.Fatal(b.Error.Code)
	}
}

func TestRequestOTPHourlyLimit(t *testing.T) {
	_, _, r := setup(t, map[string]any{"auth.otp.resend_cooldown_seconds": 0, "auth.otp.max_per_hour": 2, "auth.otp.max_per_ip_per_hour": 3})
	requestOTP(t, r, "9876543210")
	requestOTP(t, r, "9876543210")
	expect(t, testutil.Request(r, "POST", "/v1/auth/otp/request", map[string]any{"mobile": "9876543210"}, nil), 429)
	// another number from the same IP has its own mobile limit, until the (higher) IP limit is hit
	requestOTP(t, r, "9876500000")
	expect(t, testutil.Request(r, "POST", "/v1/auth/otp/request", map[string]any{"mobile": "9876500001"}, nil), 429)
}

func TestDevFixedOTPOnlyLocal(t *testing.T) {
	d, _, r := setup(t, nil)
	d.Cfg.DevFixedOTP = "111111"
	requestOTP(t, r, "9876543210")
	if len(sent(d)) != 0 {
		t.Fatal("fixed OTP must not send SMS")
	}
	d2, _, r2 := setup(t, nil)
	d2.Cfg.DevFixedOTP = "111111"
	d2.Cfg.Env = config.EnvProd
	requestOTP(t, r2, "9876543210")
	if len(sent(d2)) != 1 || sent(d2)[0].OTP == "111111" {
		t.Fatal("fixed OTP must be ignored outside local")
	}
}

func TestVerifyOTPWrongAndBurnt(t *testing.T) {
	d, _, r := setup(t, map[string]any{"auth.otp.max_attempts": 2})
	expect(t, testutil.Request(r, "POST", "/v1/auth/otp/verify", map[string]any{"requestId": "x", "otp": ""}, nil), 422)
	tk := requestOTP(t, r, "9876543210")
	code := sent(d)[0].OTP
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	b := expect(t, testutil.Request(r, "POST", "/v1/auth/otp/verify", map[string]any{"requestId": tk.RequestID, "otp": wrong}, nil), 422)
	if b.Error.Fields["otp"] != "is incorrect" {
		t.Fatal(b.Error.Fields)
	}
	b = expect(t, testutil.Request(r, "POST", "/v1/auth/otp/verify", map[string]any{"requestId": tk.RequestID, "otp": wrong}, nil), 422)
	if !strings.Contains(b.Error.Fields["otp"], "expired") {
		t.Fatal(b.Error.Fields)
	}
	// burnt: even the right code no longer works
	expect(t, testutil.Request(r, "POST", "/v1/auth/otp/verify", map[string]any{"requestId": tk.RequestID, "otp": code}, nil), 422)
}

func TestVerifyOTPNewUser(t *testing.T) {
	d, mock, r := setup(t, map[string]any{"auth.refresh_token_ttl_days": 10})
	registered := 0
	d.Dispatcher.On(events.UserRegistered, func(_ context.Context, e events.Event) error {
		var p events.UserPayload
		_ = e.Decode(&p)
		if p.UserID == 77 {
			registered++
		}
		return nil
	})
	tk := requestOTP(t, r, "9876543210")
	hash := d.Crypto.Hash("mobile", "+919876543210")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, status FROM users WHERE mobile_hash = \? FOR UPDATE`).WithArgs(hash).WillReturnRows(sqlmock.NewRows([]string{"id", "status"}))
	mock.ExpectExec(`INSERT INTO users \(mobile_enc, mobile_hash, mobile_last4, last_login_at\)`).WithArgs(sqlmock.AnyArg(), hash, "3210", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(77, 1))
	mock.ExpectExec(`INSERT INTO refresh_tokens`).WithArgs("user", 77, sqlmock.AnyArg(), sqlmock.AnyArg(), "pixel-7", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	usertest.ExpectMe(mock, usertest.MeRow{ID: 77, Step: "role", Verify: "unverified"})
	b := expect(t, testutil.Request(r, "POST", "/v1/auth/otp/verify", map[string]any{"requestId": tk.RequestID, "otp": sent(d)[0].OTP, "deviceId": "pixel-7"}, nil), 200)
	var out LoginResult
	_ = json.Unmarshal(b.Data, &out)
	if !out.IsNewUser || out.User.ID != 77 || out.Tokens.AccessToken == "" || out.Tokens.RefreshToken == "" {
		t.Fatalf("%+v", out)
	}
	if c, err := d.Tokens.ParseUser(out.Tokens.AccessToken); err != nil || c.SubjectID() != 77 {
		t.Fatal("access token", err)
	}
	if days := time.Until(out.Tokens.RefreshTokenExpiresAt).Hours() / 24; days < 9.9 || days > 10.1 {
		t.Fatalf("refresh ttl %.1f days, want 10 (auth.refresh_token_ttl_days)", days)
	}
	if registered != 1 {
		t.Fatal("user.registered not published once")
	}
	// the code is single use
	expect(t, testutil.Request(r, "POST", "/v1/auth/otp/verify", map[string]any{"requestId": tk.RequestID, "otp": sent(d)[0].OTP}, nil), 422)
}

func TestVerifyOTPExistingAndSuspended(t *testing.T) {
	d, mock, r := setup(t, map[string]any{"auth.otp.resend_cooldown_seconds": 0})
	registered := 0
	d.Dispatcher.On(events.UserRegistered, func(context.Context, events.Event) error { registered++; return nil })

	tk := requestOTP(t, r, "9876543210")
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM users WHERE mobile_hash`).WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(42, "active"))
	mock.ExpectExec(`UPDATE users SET last_login_at = \? WHERE id = \?`).WithArgs(sqlmock.AnyArg(), 42).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO refresh_tokens`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	usertest.ExpectMe(mock, usertest.MeRow{ID: 42, PrimaryRole: "player", Roles: []string{"player"}})
	b := expect(t, testutil.Request(r, "POST", "/v1/auth/otp/verify", map[string]any{"requestId": tk.RequestID, "otp": sent(d)[0].OTP}, nil), 200)
	var out LoginResult
	_ = json.Unmarshal(b.Data, &out)
	if out.IsNewUser || registered != 0 {
		t.Fatal("existing user must not be new")
	}

	tk = requestOTP(t, r, "9876543210")
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM users WHERE mobile_hash`).WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(42, "suspended"))
	mock.ExpectRollback()
	b = expect(t, testutil.Request(r, "POST", "/v1/auth/otp/verify", map[string]any{"requestId": tk.RequestID, "otp": sent(d)[1].OTP}, nil), 403)
	if b.Error.Code != "ACCOUNT_SUSPENDED" {
		t.Fatal(b.Error.Code)
	}
}

var tokenCols = []string{"id", "subject_id", "family_id", "device_id", "expires_at", "revoked_at"}

func TestRefresh(t *testing.T) {
	d, mock, r := setup(t, nil)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM refresh_tokens WHERE token_hash = \? AND subject_type = \?`).WithArgs(crypto.SHA256("good"), "user").
		WillReturnRows(sqlmock.NewRows(tokenCols).AddRow(1, 42, "fam", nil, time.Now().Add(time.Hour), nil))
	mock.ExpectQuery(`SELECT status FROM users WHERE id = \?`).WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"s"}).AddRow("active"))
	mock.ExpectExec(`INSERT INTO refresh_tokens`).WithArgs("user", 42, sqlmock.AnyArg(), "fam", nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \?, replaced_by = \?`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	b := expect(t, testutil.Request(r, "POST", "/v1/auth/refresh", map[string]any{"refreshToken": "good"}, nil), 200)
	var p tokens.Pair
	_ = json.Unmarshal(b.Data, &p)
	if c, err := d.Tokens.ParseUser(p.AccessToken); err != nil || c.SubjectID() != 42 || p.RefreshToken == "good" {
		t.Fatal(p, err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM refresh_tokens`).WillReturnRows(sqlmock.NewRows(tokenCols))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/auth/refresh", map[string]any{"refreshToken": "bad"}, nil), 401)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM refresh_tokens`).WillReturnRows(sqlmock.NewRows(tokenCols).AddRow(1, 42, "fam", nil, time.Now().Add(time.Hour), nil))
	mock.ExpectQuery(`SELECT status FROM users`).WillReturnRows(sqlmock.NewRows([]string{"s"}).AddRow("suspended"))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/auth/refresh", map[string]any{"refreshToken": "x"}, nil), 403)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM refresh_tokens`).WillReturnRows(sqlmock.NewRows(tokenCols).AddRow(1, 42, "fam", nil, time.Now().Add(time.Hour), nil))
	mock.ExpectQuery(`SELECT status FROM users`).WillReturnRows(sqlmock.NewRows([]string{"s"}).AddRow("deleted"))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/auth/refresh", map[string]any{"refreshToken": "x"}, nil), 401)
}

func TestLogout(t *testing.T) {
	_, mock, r := setup(t, nil)
	// one session family, scoped to the caller
	mock.ExpectExec(`UPDATE refresh_tokens t JOIN`).WithArgs(crypto.SHA256("tok"), "user", 42, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	expect(t, testutil.Request(r, "POST", "/v1/auth/logout", map[string]any{"refreshToken": "tok"}, nil), 204)
	// no body: all sessions
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \? WHERE subject_type = \? AND subject_id = \?`).WithArgs(sqlmock.AnyArg(), "user", 42).WillReturnResult(sqlmock.NewResult(0, 3))
	expect(t, testutil.Request(r, "POST", "/v1/auth/logout", nil, nil), 204)
}

func TestCleanupJob(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	m := New(d)
	mock.ExpectExec(`DELETE FROM refresh_tokens WHERE expires_at < \? OR revoked_at < \? LIMIT \?`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 500).WillReturnResult(sqlmock.NewResult(0, 500))
	mock.ExpectExec(`DELETE FROM refresh_tokens`).WillReturnResult(sqlmock.NewResult(0, 3))
	if err := m.Jobs()["auth.cleanup_tokens"](context.Background()); err != nil {
		t.Fatal(err)
	}
}

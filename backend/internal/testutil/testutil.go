// Package testutil builds a fully wired *app.Deps for unit tests with no network:
// sqlmock DB, miniredis, static settings, random crypto/JWT keys, in-memory storage,
// synchronous event bus, stub SMS.
//
//	d, mock := testutil.Deps(t, map[string]any{"chat.group_max_members": 3})
//	mock.ExpectQuery(`SELECT .* FROM events WHERE id = \? AND organiser_id = \?`).WithArgs(7, 42).WillReturnRows(...)
package testutil

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/config"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/eligibility"
	"athlix/backend/internal/platform/entitlements"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/media"
	"athlix/backend/internal/platform/ratelimit"
	"athlix/backend/internal/platform/requirements"
	"athlix/backend/internal/platform/secrets"
	"athlix/backend/internal/platform/settings"
	"athlix/backend/internal/platform/storage"
	"athlix/backend/internal/platform/userdir"
)

func key() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// Deps returns test dependencies. Unmet sqlmock expectations fail the test at cleanup.
func Deps(t *testing.T, cfgValues map[string]any) (*app.Deps, sqlmock.Sqlmock) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	box, err := crypto.NewBox(crypto.Keys{Current: 1, Keys: map[string]string{"1": key()}, HMACKey: key()})
	if err != nil {
		t.Fatal(err)
	}
	iss, err := authn.NewIssuer(authn.SigningKeys{UserSigningKey: key(), AdminSigningKey: key()}, "athlix-test")
	if err != nil {
		t.Fatal(err)
	}
	if cfgValues == nil {
		cfgValues = map[string]any{}
	}
	st := settings.NewStatic(cfgValues)
	store := NewMemStorage()
	disp := events.NewDispatcher()
	d := &app.Deps{
		Cfg:          &config.Config{Env: config.EnvLocal, SecretPrefix: "athlix/test", PublicAPIBaseURL: "http://test", AppDeepLinkBase: "athlix://", WSPublicURL: "ws://test/ws"},
		DB:           conn,
		Redis:        rdb,
		Settings:     st,
		Crypto:       box,
		Secrets:      secrets.Static{},
		Storage:      store,
		Media:        media.New(conn, store, st),
		SMS:          &RecordingSMS{},
		Events:       &events.Local{D: disp, Sync: true},
		Dispatcher:   disp,
		Users:        userdir.New(conn, nil),
		Entitlements: entitlements.New(conn),
		Requirements: requirements.NewStatic(DefaultRequirements),
		Eligibility:  eligibility.NewWithPlaces(conn, st, eligibility.TestPlaces()),
		Tokens:       iss,
		Limiter:      ratelimit.New(rdb),
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:          time.Now,
	}
	t.Cleanup(func() {
		_ = rdb.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("sql expectations: %v", err)
		}
	})
	return d, mock
}

// RecordingSMS captures sent OTPs (tests only).
type RecordingSMS struct {
	mu   sync.Mutex
	Sent []SentSMS
}

// SentSMS is one captured message.
type SentSMS struct{ Template, Mobile, OTP string }

func (r *RecordingSMS) SendOTP(_ context.Context, template, mobile, otp string) error {
	r.mu.Lock()
	r.Sent = append(r.Sent, SentSMS{template, mobile, otp})
	r.mu.Unlock()
	return nil
}

// MemStorage is an in-memory storage.Storage.
type MemStorage struct {
	mu      sync.Mutex
	Objects map[string]storage.Object
}

// NewMemStorage builds an empty MemStorage.
func NewMemStorage() *MemStorage { return &MemStorage{Objects: map[string]storage.Object{}} }

// Put simulates a client upload.
func (m *MemStorage) Put(key, contentType string, size int64) {
	m.mu.Lock()
	m.Objects[key] = storage.Object{Size: size, ContentType: contentType}
	m.mu.Unlock()
}

func (m *MemStorage) PresignPut(_ context.Context, key, ct string, size int64, _ time.Duration) (string, map[string]string, error) {
	return "https://storage.test/put/" + key, map[string]string{"Content-Type": ct}, nil
}
func (m *MemStorage) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.test/get/" + key, nil
}
func (m *MemStorage) Head(_ context.Context, key string) (storage.Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.Objects[key]
	if !ok {
		return storage.Object{}, storage.ErrNotFound
	}
	return o, nil
}
func (m *MemStorage) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.Objects, key)
	m.mu.Unlock()
	return nil
}

// Request runs one request through a gin engine and returns the recorder.
func Request(r http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// UserEngine returns a gin engine whose requests are pre-authenticated as u (skips JWT +
// userdir), with group helpers mirroring app.Routes. Use it to test a module's handlers.
func UserEngine(u userdir.Basic) (*gin.Engine, app.Routes) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/v1")
	asUser := func(c *gin.Context) { authn.SetTestUser(c, u); c.Next() }
	user := v1.Group("", asUser)
	return r, app.Routes{Public: v1.Group(""), User: user, Verified: user, AdminPublic: v1.Group("/admin"), Admin: v1.Group("/admin"), Webhooks: v1.Group("/webhooks"), Engine: r}
}

// AdminEngine returns a gin engine pre-authenticated as an admin with perms.
func AdminEngine(adminID int64, perms []string) (*gin.Engine, app.Routes) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/v1")
	asAdmin := func(c *gin.Context) { authn.SetTestAdmin(c, adminID, perms); c.Next() }
	return r, app.Routes{Public: v1.Group(""), User: v1.Group(""), Verified: v1.Group(""), AdminPublic: v1.Group("/admin"), Admin: v1.Group("/admin", asAdmin), Webhooks: v1.Group("/webhooks"), Engine: r}
}

// VerifiedUser is a ready-made onboarded adult player.
func VerifiedUser(id int64) userdir.Basic {
	return userdir.Basic{ID: id, DisplayName: "Test User", PrimaryRole: "player", Roles: []string{"player"}, Status: "active", OnboardingStep: "done", VerificationStatus: "verified", GuardianConsentStatus: "not_required"}
}

// DefaultRequirements mirrors the seed rows in migrations/0007_seed.sql (verification_requirements).
var DefaultRequirements = map[string]bool{
	"player/digilocker/app_access":                    true,
	"coach/digilocker/app_access":                     true,
	"organiser/digilocker/app_access":                 true,
	"player/guardian_consent_if_minor/app_access":     true,
	"coach/guardian_consent_if_minor/app_access":      true,
	"organiser/guardian_consent_if_minor/app_access":  true,
	"organiser/organiser_approval/create_event":       true,
	"coach/coach_pro_subscription/offer_consultation": false,
}

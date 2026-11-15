package push

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"athlix/backend/internal/platform/secrets"
)

var (
	keyOnce sync.Once
	testKey *rsa.PrivateKey
)

func rsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		testKey = k
	})
	return testKey
}

func pkcs8PEM(t *testing.T) string {
	b, err := x509.MarshalPKCS8PrivateKey(rsaKey(t))
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b}))
}

func pkcs1PEM(t *testing.T) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey(t))}))
}

const (
	deviceToken = "device-token-SECRET-123"
	accessTok   = "ya29.ACCESS-SECRET"
)

// fakeGoogle serves the OAuth token endpoint and the FCM send endpoint.
type fakeGoogle struct {
	t          *testing.T
	mu         sync.Mutex
	tokenCalls int
	sendCalls  int
	tokens     []string // access tokens handed out, in order
	sendStatus []int    // status per send call (default 200)
	sendBody   string
	lastSend   map[string]any
	lastAuth   string
	assertions []string
}

func (g *fakeGoogle) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		g.mu.Lock()
		defer g.mu.Unlock()
		if r.Form.Get("grant_type") != jwtGrantType {
			w.WriteHeader(400)
			return
		}
		g.assertions = append(g.assertions, r.Form.Get("assertion"))
		tok := accessTok
		if g.tokenCalls < len(g.tokens) {
			tok = g.tokens[g.tokenCalls]
		}
		g.tokenCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": tok, "expires_in": 3600, "token_type": "Bearer"})
	})
	mux.HandleFunc("/v1/projects/proj-1/messages:send", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		g.mu.Lock()
		defer g.mu.Unlock()
		g.lastAuth = r.Header.Get("Authorization")
		g.lastSend = map[string]any{}
		_ = json.Unmarshal(b, &g.lastSend)
		status := 200
		if g.sendCalls < len(g.sendStatus) {
			status = g.sendStatus[g.sendCalls]
		}
		g.sendCalls++
		w.WriteHeader(status)
		if status != 200 {
			_, _ = io.WriteString(w, g.sendBody)
			return
		}
		_, _ = io.WriteString(w, `{"name":"projects/proj-1/messages/1"}`)
	})
	return mux
}

func newTestFCM(t *testing.T, g *fakeGoogle, now func() time.Time) *FCM {
	t.Helper()
	srv := httptest.NewServer(g.handler())
	t.Cleanup(srv.Close)
	opts := []Option{WithTokenURL(srv.URL + "/token"), WithSendBaseURL(srv.URL + "/"), WithHTTPClient(srv.Client())}
	if now != nil {
		opts = append(opts, WithClock(now))
	}
	f, err := NewFCM(Credentials{ProjectID: "proj-1", ClientEmail: "svc@proj-1.iam.gserviceaccount.com", PrivateKey: pkcs8PEM(t)}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func msg() Message {
	return Message{Token: deviceToken, Title: "Hi", Body: "There", Data: map[string]string{"eventId": "9"}}
}

func TestSendCachesAccessToken(t *testing.T) {
	g := &fakeGoogle{t: t}
	f := newTestFCM(t, g, nil)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := f.Send(ctx, msg()); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if g.tokenCalls != 1 || g.sendCalls != 2 {
		t.Fatalf("tokenCalls=%d sendCalls=%d", g.tokenCalls, g.sendCalls)
	}
	if g.lastAuth != "Bearer "+accessTok {
		t.Fatalf("auth header = %q", g.lastAuth)
	}
	m := g.lastSend["message"].(map[string]any)
	if m["token"] != deviceToken || m["notification"].(map[string]any)["title"] != "Hi" || m["data"].(map[string]any)["eventId"] != "9" {
		t.Fatalf("unexpected body %v", g.lastSend)
	}
	if m["android"].(map[string]any)["priority"] != "high" {
		t.Fatalf("android priority missing")
	}

	// The assertion is an RS256 JWT for the service account with the FCM scope.
	parsed, err := jwt.Parse(g.assertions[0], func(*jwt.Token) (any, error) { return &rsaKey(t).PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		t.Fatal(err)
	}
	c := parsed.Claims.(jwt.MapClaims)
	if c["iss"] != "svc@proj-1.iam.gserviceaccount.com" || c["scope"] != fcmScope || !strings.HasSuffix(c["aud"].(string), "/token") {
		t.Fatalf("claims = %v", c)
	}
}

func TestTokenRefetchedAfterExpiry(t *testing.T) {
	g := &fakeGoogle{t: t}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := newTestFCM(t, g, func() time.Time { return now })
	ctx := context.Background()
	_ = f.Send(ctx, msg())
	now = now.Add(58 * time.Minute) // still before expiry-60s
	_ = f.Send(ctx, msg())
	if g.tokenCalls != 1 {
		t.Fatalf("tokenCalls=%d, want 1", g.tokenCalls)
	}
	now = now.Add(2 * time.Minute) // past expiry-60s
	_ = f.Send(ctx, msg())
	if g.tokenCalls != 2 {
		t.Fatalf("tokenCalls=%d, want 2", g.tokenCalls)
	}
}

func TestUnauthorizedRefreshesOnce(t *testing.T) {
	g := &fakeGoogle{t: t, tokens: []string{"old-token", "new-token"}, sendStatus: []int{401, 200}}
	f := newTestFCM(t, g, nil)
	if err := f.Send(context.Background(), msg()); err != nil {
		t.Fatal(err)
	}
	if g.tokenCalls != 2 || g.sendCalls != 2 || g.lastAuth != "Bearer new-token" {
		t.Fatalf("tokenCalls=%d sendCalls=%d auth=%q", g.tokenCalls, g.sendCalls, g.lastAuth)
	}

	// Two 401s in a row: give up after one retry.
	g2 := &fakeGoogle{t: t, sendStatus: []int{401, 401, 401}}
	f2 := newTestFCM(t, g2, nil)
	err := f2.Send(context.Background(), msg())
	if err == nil || g2.sendCalls != 2 {
		t.Fatalf("err=%v sendCalls=%d", err, g2.sendCalls)
	}
}

func TestUnregisteredMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"404", 404, `{"error":{"status":"NOT_FOUND"}}`, ErrUnregistered},
		{"400 unregistered", 400, `{"error":{"status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`, ErrUnregistered},
		{"400 token field", 400, `{"error":{"status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.rpc.BadRequest","fieldViolations":[{"field":"message.token"}]}]}}`, ErrUnregistered},
		{"400 token message", 400, `{"error":{"status":"INVALID_ARGUMENT","message":"The registration token is not a valid FCM registration token"}}`, ErrUnregistered},
		{"403 sender mismatch", 403, `{"error":{"status":"PERMISSION_DENIED","details":[{"errorCode":"SENDER_ID_MISMATCH"}]}}`, ErrUnregistered},
		{"400 other field", 400, `{"error":{"status":"INVALID_ARGUMENT","details":[{"fieldViolations":[{"field":"message.data"}]}]}}`, nil},
		{"503", 503, `{"error":{"status":"UNAVAILABLE"}}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakeGoogle{t: t, sendStatus: []int{tc.status}, sendBody: tc.body}
			err := newTestFCM(t, g, nil).Send(context.Background(), msg())
			if tc.want != nil {
				if !errors.Is(err, tc.want) {
					t.Fatalf("err = %v, want %v", err, tc.want)
				}
				return
			}
			if err == nil || errors.Is(err, ErrUnregistered) {
				t.Fatalf("err = %v, want generic failure", err)
			}
		})
	}
}

func TestErrorsNeverContainSecrets(t *testing.T) {
	g := &fakeGoogle{t: t, sendStatus: []int{503}, sendBody: `{"error":{"message":"` + deviceToken + `"}}`}
	err := newTestFCM(t, g, nil).Send(context.Background(), msg())
	if err == nil {
		t.Fatal("expected error")
	}
	s := err.Error()
	for _, secret := range []string{deviceToken, accessTok, "PRIVATE KEY"} {
		if strings.Contains(s, secret) {
			t.Fatalf("error leaks %q: %s", secret, s)
		}
	}
	if s != "push: fcm send failed (status 503)" {
		t.Fatalf("unexpected error text %q", s)
	}

	// Auth failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"`+accessTok+`"}`)
	}))
	defer srv.Close()
	f, _ := NewFCM(Credentials{ProjectID: "proj-1", ClientEmail: "svc@x", PrivateKey: pkcs8PEM(t)}, WithTokenURL(srv.URL), WithSendBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	err = f.Send(context.Background(), msg())
	if err == nil || strings.Contains(err.Error(), accessTok) || strings.Contains(err.Error(), deviceToken) {
		t.Fatalf("auth err = %v", err)
	}
}

func TestNewFCMKeyFormats(t *testing.T) {
	for name, k := range map[string]string{"pkcs8": pkcs8PEM(t), "pkcs1": pkcs1PEM(t), "escaped": strings.ReplaceAll(pkcs8PEM(t), "\n", `\n`)} {
		if _, err := NewFCM(Credentials{ProjectID: "p", ClientEmail: "e", PrivateKey: k}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := NewFCM(Credentials{ProjectID: "p", ClientEmail: "e", PrivateKey: "not a key"}); err == nil {
		t.Fatal("expected error for bad key")
	}
	if _, err := NewFCM(Credentials{ClientEmail: "e", PrivateKey: pkcs8PEM(t)}); err == nil {
		t.Fatal("expected error for missing project")
	}
	_, err := NewFCM(Credentials{ProjectID: "p", ClientEmail: "e", PrivateKey: fakePEM("PRIVATE KEY", "AAAA")})
	if err == nil || strings.Contains(err.Error(), "AAAA") {
		t.Fatalf("err = %v", err)
	}
}

func TestEmptyTokenIsUnregistered(t *testing.T) {
	g := &fakeGoogle{t: t}
	if err := newTestFCM(t, g, nil).Send(context.Background(), Message{}); !errors.Is(err, ErrUnregistered) {
		t.Fatalf("err = %v", err)
	}
	if g.tokenCalls != 0 {
		t.Fatal("should not call google for empty token")
	}
}

func TestFromSecrets(t *testing.T) {
	creds := Credentials{ProjectID: "p", ClientEmail: "e", PrivateKey: pkcs8PEM(t)}
	s, err := FromSecrets(context.Background(), secrets.Static{"athlix/test/fcm": creds}, "athlix/test/fcm")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*FCM); !ok {
		t.Fatalf("got %T", s)
	}
	if _, err := FromSecrets(context.Background(), secrets.Static{}, "athlix/test/fcm"); err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestFakeAndNop(t *testing.T) {
	f := &Fake{Fail: map[string]error{"bad": ErrUnregistered}}
	if err := f.Send(context.Background(), Message{Token: "good"}); err != nil {
		t.Fatal(err)
	}
	if err := f.Send(context.Background(), Message{Token: "bad"}); !errors.Is(err, ErrUnregistered) {
		t.Fatal(err)
	}
	if got := f.Messages(); len(got) != 1 || got[0].Token != "good" {
		t.Fatalf("got %v", got)
	}
	if err := (Nop{}).Send(context.Background(), Message{}); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	var _ Sender = (*FCM)(nil)
}

// fakePEM builds an invalid PEM block at runtime so secret scanners do not flag the test source.
func fakePEM(kind, body string) string {
	return "-----BEGIN " + kind + "-----\n" + body + "\n-----END " + kind + "-----\n"
}

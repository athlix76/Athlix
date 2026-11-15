package push

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// DefaultTokenURL is Google's OAuth 2.0 token endpoint.
	DefaultTokenURL = "https://oauth2.googleapis.com/token"
	// DefaultSendBaseURL is the FCM HTTP v1 API host.
	DefaultSendBaseURL = "https://fcm.googleapis.com"

	fcmScope       = "https://www.googleapis.com/auth/firebase.messaging"
	jwtGrantType   = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	tokenLifetime  = time.Hour
	tokenEarlyLeft = 60 * time.Second
	maxBody        = 64 << 10
)

// Option customises an FCM client (tests point it at httptest servers).
type Option func(*FCM)

// WithTokenURL overrides the OAuth token endpoint.
func WithTokenURL(u string) Option { return func(f *FCM) { f.tokenURL = u } }

// WithSendBaseURL overrides the FCM API base URL (scheme + host, no trailing slash).
func WithSendBaseURL(u string) Option {
	return func(f *FCM) { f.sendBase = strings.TrimRight(u, "/") }
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(c *http.Client) Option { return func(f *FCM) { f.hc = c } }

// WithClock overrides the clock used for JWT times and token expiry.
func WithClock(now func() time.Time) Option { return func(f *FCM) { f.now = now } }

// FCM is a Sender for Firebase Cloud Messaging HTTP v1. The OAuth access token is cached in
// memory only, until 60 s before it expires.
type FCM struct {
	projectID   string
	clientEmail string
	key         *rsa.PrivateKey
	tokenURL    string
	sendBase    string
	hc          *http.Client
	now         func() time.Time

	mu      sync.Mutex
	token   string
	expires time.Time
}

// NewFCM validates the credentials and parses the service-account private key (PKCS#8 or
// PKCS#1 PEM). Errors never contain key material.
func NewFCM(c Credentials, opts ...Option) (*FCM, error) {
	if strings.TrimSpace(c.ProjectID) == "" || strings.TrimSpace(c.ClientEmail) == "" {
		return nil, errors.New("push: fcm credentials are incomplete")
	}
	key, err := parseRSAKey(c.PrivateKey)
	if err != nil {
		return nil, err
	}
	f := &FCM{
		projectID:   c.ProjectID,
		clientEmail: c.ClientEmail,
		key:         key,
		tokenURL:    DefaultTokenURL,
		sendBase:    DefaultSendBaseURL,
		hc:          &http.Client{Timeout: 10 * time.Second},
		now:         time.Now,
	}
	for _, o := range opts {
		o(f)
	}
	return f, nil
}

func parseRSAKey(pemText string) (*rsa.PrivateKey, error) {
	// Secrets pasted by hand sometimes keep the JSON "\n" escapes literally.
	if !strings.Contains(pemText, "\n") && strings.Contains(pemText, `\n`) {
		pemText = strings.ReplaceAll(pemText, `\n`, "\n")
	}
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("push: fcm private key is not PEM")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("push: fcm private key is not RSA")
		}
		return rk, nil
	}
	k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("push: fcm private key cannot be parsed")
	}
	return k, nil
}

// Send implements Sender. A 401 drops the cached access token and retries once.
func (f *FCM) Send(ctx context.Context, m Message) error {
	if strings.TrimSpace(m.Token) == "" {
		return ErrUnregistered
	}
	body, err := json.Marshal(fcmRequest(m))
	if err != nil {
		return errors.New("push: cannot encode message")
	}
	for attempt := 0; ; attempt++ {
		tok, err := f.accessToken(ctx)
		if err != nil {
			return err
		}
		status, resp, err := f.post(ctx, tok, body)
		if err != nil {
			return err
		}
		if status == http.StatusUnauthorized && attempt == 0 {
			f.dropToken(tok)
			continue
		}
		return classify(status, resp)
	}
}

func fcmRequest(m Message) map[string]any {
	msg := map[string]any{
		"token":        m.Token,
		"notification": map[string]string{"title": m.Title, "body": m.Body},
		"android":      map[string]string{"priority": "high"},
		"apns":         map[string]any{"payload": map[string]any{"aps": map[string]string{"sound": "default"}}},
	}
	if len(m.Data) > 0 {
		msg["data"] = m.Data
	}
	return map[string]any{"message": msg}
}

func (f *FCM) post(ctx context.Context, accessToken string, body []byte) (int, []byte, error) {
	u := f.sendBase + "/v1/projects/" + url.PathEscape(f.projectID) + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return 0, nil, errors.New("push: cannot build fcm request")
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := f.hc.Do(req)
	if err != nil {
		return 0, nil, errors.New("push: fcm send failed (network)")
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))
	return res.StatusCode, b, nil
}

// fcmError is the subset of the Google API error body used to classify failures. It is only
// inspected, never logged or returned.
type fcmError struct {
	Error struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Details []struct {
			ErrorCode       string `json:"errorCode"`
			FieldViolations []struct {
				Field string `json:"field"`
			} `json:"fieldViolations"`
		} `json:"details"`
	} `json:"error"`
}

func classify(status int, body []byte) error {
	if status >= 200 && status < 300 {
		return nil
	}
	if status == http.StatusNotFound {
		return ErrUnregistered
	}
	if status == http.StatusBadRequest || status == http.StatusForbidden {
		var e fcmError
		if json.Unmarshal(body, &e) == nil {
			for _, d := range e.Error.Details {
				if d.ErrorCode == "UNREGISTERED" || d.ErrorCode == "SENDER_ID_MISMATCH" || tokenField(d.FieldViolations) {
					return ErrUnregistered
				}
			}
			if e.Error.Status == "INVALID_ARGUMENT" && strings.Contains(strings.ToLower(e.Error.Message), "registration token") {
				return ErrUnregistered
			}
		}
	}
	return fmt.Errorf("push: fcm send failed (status %d)", status)
}

func tokenField(vs []struct {
	Field string `json:"field"`
}) bool {
	for _, v := range vs {
		if v.Field == "message.token" {
			return true
		}
	}
	return false
}

func (f *FCM) dropToken(tok string) {
	f.mu.Lock()
	if f.token == tok {
		f.token = ""
		f.expires = time.Time{}
	}
	f.mu.Unlock()
}

// accessToken returns the cached OAuth token or fetches a new one with a signed
// service-account JWT. The lock is held during the fetch so concurrent sends share one fetch.
func (f *FCM) accessToken(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	if f.token != "" && now.Before(f.expires) {
		return f.token, nil
	}
	claims := jwt.MapClaims{
		"iss":   f.clientEmail,
		"scope": fcmScope,
		"aud":   f.tokenURL,
		"iat":   now.Unix(),
		"exp":   now.Add(tokenLifetime).Unix(),
	}
	assertion, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(f.key)
	if err != nil {
		return "", errors.New("push: cannot sign fcm assertion")
	}
	form := url.Values{"grant_type": {jwtGrantType}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", errors.New("push: cannot build fcm auth request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := f.hc.Do(req)
	if err != nil {
		return "", errors.New("push: fcm auth failed (network)")
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("push: fcm auth failed (status %d)", res.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if json.Unmarshal(b, &out) != nil || out.AccessToken == "" {
		return "", errors.New("push: fcm auth response is invalid")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = int64(tokenLifetime / time.Second)
	}
	f.token = out.AccessToken
	f.expires = now.Add(time.Duration(out.ExpiresIn)*time.Second - tokenEarlyLeft)
	return f.token, nil
}

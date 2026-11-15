package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"athlix/backend/internal/platform/secrets"
)

// Store errors. They never carry tokens, URLs or response bodies.
var (
	errStoreDisabled = errors.New("store: not configured")
	errUpstream      = errors.New("store: request failed")
	errNotFound      = errors.New("store: purchase not found")
	errBadSignature  = errors.New("store: signature check failed")
)

// PlaySubscription is the server-side state of one Google Play purchase token.
type PlaySubscription struct {
	ProductID           string
	State               string // SUBSCRIPTION_STATE_*
	ObfuscatedAccountID string
	StartedAt           time.Time
	ExpiresAt           time.Time
	AutoRenew           bool
	LinkedPurchaseToken string
	Acknowledged        bool
}

// PlayStore talks to the Google Play Developer API.
type PlayStore interface {
	PackageName(ctx context.Context) (string, error)
	Subscription(ctx context.Context, purchaseToken string) (PlaySubscription, error)
	Acknowledge(ctx context.Context, productID, purchaseToken string) error
}

// PushVerifier authenticates a Pub/Sub push request (the Authorization header value).
type PushVerifier interface {
	Verify(ctx context.Context, authorization string) error
}

// AppleSubscription is one auto-renewable subscription from the App Store Server API.
type AppleSubscription struct {
	OriginalTransactionID string
	TransactionID         string
	ProductID             string
	AppAccountToken       string
	Status                int // 1 active, 2 expired, 3 billing retry, 4 grace, 5 revoked
	StartedAt             time.Time
	ExpiresAt             time.Time
	GraceExpiresAt        *time.Time
	AutoRenew             bool
	Revoked               bool
}

// AppleNotification is a verified App Store Server Notification v2.
type AppleNotification struct {
	UUID                  string
	Type                  string
	TransactionID         string
	OriginalTransactionID string
	Ours                  bool // bundle id and environment match our app-store secret
}

// AppleStore talks to the App Store Server API and verifies notifications.
type AppleStore interface {
	Statuses(ctx context.Context, transactionID string) ([]AppleSubscription, error)
	VerifyNotification(ctx context.Context, signedPayload string) (AppleNotification, error)
}

type googlePlaySecret struct {
	PackageName               string `json:"packageName"`
	ClientEmail               string `json:"clientEmail"`
	PrivateKey                string `json:"privateKey"`
	PubsubAudience            string `json:"pubsubAudience"`
	PubsubServiceAccountEmail string `json:"pubsubServiceAccountEmail"`
}

type appStoreSecret struct {
	BundleID    string          `json:"bundleId"`
	IssuerID    string          `json:"issuerId"`
	KeyID       string          `json:"keyId"`
	PrivateKey  string          `json:"privateKey"`
	Environment string          `json:"environment"`
	AppAppleID  json.RawMessage `json:"appAppleId"`
}

const maxStoreResponse = 1 << 20

// doJSON performs req and decodes a 2xx JSON body into out (nil = ignore body).
// 404/410 map to errNotFound; anything else to errUpstream. The error never includes the URL.
func doJSON(hc *http.Client, req *http.Request, out any) error {
	res, err := hc.Do(req)
	if err != nil {
		return errUpstream
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return errNotFound
	case res.StatusCode < 200 || res.StatusCode > 299:
		return errUpstream
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxStoreResponse)).Decode(out); err != nil {
		return errUpstream
	}
	return nil
}

// ---- Google Play ----

const (
	googleTokenURL   = "https://oauth2.googleapis.com/token"
	googlePlayAPI    = "https://androidpublisher.googleapis.com"
	androidPublisher = "https://www.googleapis.com/auth/androidpublisher"
)

type playClient struct {
	secrets  secrets.Getter
	name     string
	http     *http.Client
	tokenURL string
	apiBase  string
	now      func() time.Time

	mu       sync.Mutex
	access   string // OAuth access token, memory only
	accessTo time.Time
}

func (p *playClient) creds(ctx context.Context) (googlePlaySecret, error) {
	var s googlePlaySecret
	if err := p.secrets.GetJSON(ctx, p.name, &s); err != nil || s.PackageName == "" || s.ClientEmail == "" || s.PrivateKey == "" {
		return s, errStoreDisabled
	}
	return s, nil
}

func (p *playClient) PackageName(ctx context.Context) (string, error) {
	s, err := p.creds(ctx)
	return s.PackageName, err
}

// accessToken exchanges a service-account JWT assertion for an OAuth token (cached in memory).
func (p *playClient) accessToken(ctx context.Context, s googlePlaySecret) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if p.access != "" && now.Before(p.accessTo) {
		return p.access, nil
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(s.PrivateKey))
	if err != nil {
		return "", errStoreDisabled
	}
	assertion, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": s.ClientEmail, "scope": androidPublisher, "aud": googleTokenURL,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}).SignedString(key)
	if err != nil {
		return "", errStoreDisabled
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", errUpstream
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := doJSON(p.http, req, &out); err != nil || out.AccessToken == "" {
		return "", errUpstream
	}
	p.access = out.AccessToken
	p.accessTo = now.Add(time.Duration(out.ExpiresIn)*time.Second - time.Minute)
	return p.access, nil
}

type playV2 struct {
	StartTime                  string `json:"startTime"`
	SubscriptionState          string `json:"subscriptionState"`
	LinkedPurchaseToken        string `json:"linkedPurchaseToken"`
	AcknowledgementState       string `json:"acknowledgementState"`
	ExternalAccountIdentifiers struct {
		ObfuscatedExternalAccountID string `json:"obfuscatedExternalAccountId"`
	} `json:"externalAccountIdentifiers"`
	LineItems []struct {
		ProductID        string `json:"productId"`
		ExpiryTime       string `json:"expiryTime"`
		AutoRenewingPlan *struct {
			AutoRenewEnabled bool `json:"autoRenewEnabled"`
		} `json:"autoRenewingPlan"`
	} `json:"lineItems"`
}

func (p *playClient) Subscription(ctx context.Context, token string) (PlaySubscription, error) {
	s, err := p.creds(ctx)
	if err != nil {
		return PlaySubscription{}, err
	}
	at, err := p.accessToken(ctx, s)
	if err != nil {
		return PlaySubscription{}, err
	}
	u := p.apiBase + "/androidpublisher/v3/applications/" + url.PathEscape(s.PackageName) +
		"/purchases/subscriptionsv2/tokens/" + url.PathEscape(token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return PlaySubscription{}, errUpstream
	}
	req.Header.Set("Authorization", "Bearer "+at)
	var v playV2
	if err := doJSON(p.http, req, &v); err != nil {
		return PlaySubscription{}, err
	}
	return v.toState()
}

func (v playV2) toState() (PlaySubscription, error) {
	out := PlaySubscription{
		State:               v.SubscriptionState,
		ObfuscatedAccountID: v.ExternalAccountIdentifiers.ObfuscatedExternalAccountID,
		LinkedPurchaseToken: v.LinkedPurchaseToken,
		Acknowledged:        v.AcknowledgementState == "ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED",
	}
	if t, err := time.Parse(time.RFC3339Nano, v.StartTime); err == nil {
		out.StartedAt = t.UTC()
	}
	// One product per purchase in our catalogue; with several line items the latest expiry wins.
	for _, li := range v.LineItems {
		t, err := time.Parse(time.RFC3339Nano, li.ExpiryTime)
		if err != nil || !t.After(out.ExpiresAt) {
			continue
		}
		out.ExpiresAt, out.ProductID = t.UTC(), li.ProductID
		out.AutoRenew = li.AutoRenewingPlan != nil && li.AutoRenewingPlan.AutoRenewEnabled
	}
	if out.ProductID == "" {
		return out, errUpstream
	}
	return out, nil
}

func (p *playClient) Acknowledge(ctx context.Context, productID, token string) error {
	s, err := p.creds(ctx)
	if err != nil {
		return err
	}
	at, err := p.accessToken(ctx, s)
	if err != nil {
		return err
	}
	u := p.apiBase + "/androidpublisher/v3/applications/" + url.PathEscape(s.PackageName) +
		"/purchases/subscriptions/" + url.PathEscape(productID) + "/tokens/" + url.PathEscape(token) + ":acknowledge"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader("{}"))
	if err != nil {
		return errUpstream
	}
	req.Header.Set("Authorization", "Bearer "+at)
	req.Header.Set("Content-Type", "application/json")
	return doJSON(p.http, req, nil)
}

// ---- App Store ----

var appleAPIBase = map[string]string{
	"Production": "https://api.storekit.itunes.apple.com",
	"Sandbox":    "https://api.storekit-sandbox.itunes.apple.com",
}

type appleClient struct {
	secrets secrets.Getter
	name    string
	http    *http.Client
	bases   map[string]string // environment -> API base URL
	jws     *jwsVerifier
	now     func() time.Time
}

func (a *appleClient) creds(ctx context.Context) (appStoreSecret, error) {
	var s appStoreSecret
	if err := a.secrets.GetJSON(ctx, a.name, &s); err != nil || s.BundleID == "" || s.IssuerID == "" || s.KeyID == "" || s.PrivateKey == "" {
		return s, errStoreDisabled
	}
	if s.Environment == "" {
		s.Environment = "Production"
	}
	if _, ok := a.bases[s.Environment]; !ok {
		return s, errStoreDisabled
	}
	return s, nil
}

// apiToken is the ES256 bearer token for the App Store Server API (5 minute lifetime).
func (a *appleClient) apiToken(s appStoreSecret) (string, error) {
	key, err := jwt.ParseECPrivateKeyFromPEM([]byte(s.PrivateKey))
	if err != nil {
		return "", errStoreDisabled
	}
	now := a.now()
	t := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": s.IssuerID, "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
		"aud": "appstoreconnect-v1", "bid": s.BundleID,
	})
	t.Header["kid"] = s.KeyID
	signed, err := t.SignedString(key)
	if err != nil {
		return "", errStoreDisabled
	}
	return signed, nil
}

type appleTxnPayload struct {
	TransactionID         string `json:"transactionId"`
	OriginalTransactionID string `json:"originalTransactionId"`
	BundleID              string `json:"bundleId"`
	ProductID             string `json:"productId"`
	OriginalPurchaseDate  int64  `json:"originalPurchaseDate"`
	ExpiresDate           int64  `json:"expiresDate"`
	RevocationDate        int64  `json:"revocationDate"`
	AppAccountToken       string `json:"appAccountToken"`
	Environment           string `json:"environment"`
}

type appleRenewalPayload struct {
	AutoRenewStatus        int   `json:"autoRenewStatus"`
	GracePeriodExpiresDate int64 `json:"gracePeriodExpiresDate"`
}

func ms(v int64) time.Time { return time.UnixMilli(v).UTC() }

func (a *appleClient) Statuses(ctx context.Context, transactionID string) ([]AppleSubscription, error) {
	s, err := a.creds(ctx)
	if err != nil {
		return nil, err
	}
	tok, err := a.apiToken(s)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		a.bases[s.Environment]+"/inApps/v1/subscriptions/"+url.PathEscape(transactionID), nil)
	if err != nil {
		return nil, errUpstream
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	var res struct {
		Data []struct {
			LastTransactions []struct {
				OriginalTransactionID string `json:"originalTransactionId"`
				Status                int    `json:"status"`
				SignedTransactionInfo string `json:"signedTransactionInfo"`
				SignedRenewalInfo     string `json:"signedRenewalInfo"`
			} `json:"lastTransactions"`
		} `json:"data"`
	}
	if err := doJSON(a.http, req, &res); err != nil {
		return nil, err
	}
	out := []AppleSubscription{}
	for _, g := range res.Data {
		for _, lt := range g.LastTransactions {
			var tx appleTxnPayload
			if err := a.jws.verify(lt.SignedTransactionInfo, &tx); err != nil {
				return nil, errUpstream // a response we cannot verify is never trusted
			}
			if tx.BundleID != s.BundleID {
				continue
			}
			var rn appleRenewalPayload
			if lt.SignedRenewalInfo != "" {
				if err := a.jws.verify(lt.SignedRenewalInfo, &rn); err != nil {
					return nil, errUpstream
				}
			}
			sub := AppleSubscription{
				OriginalTransactionID: tx.OriginalTransactionID, TransactionID: tx.TransactionID,
				ProductID: tx.ProductID, AppAccountToken: tx.AppAccountToken, Status: lt.Status,
				StartedAt: ms(tx.OriginalPurchaseDate), ExpiresAt: ms(tx.ExpiresDate),
				AutoRenew: rn.AutoRenewStatus == 1, Revoked: tx.RevocationDate > 0,
			}
			if rn.GracePeriodExpiresDate > 0 {
				g := ms(rn.GracePeriodExpiresDate)
				sub.GraceExpiresAt = &g
			}
			out = append(out, sub)
		}
	}
	return out, nil
}

func (a *appleClient) VerifyNotification(ctx context.Context, signedPayload string) (AppleNotification, error) {
	var p struct {
		NotificationType string `json:"notificationType"`
		Subtype          string `json:"subtype"`
		NotificationUUID string `json:"notificationUUID"`
		Data             struct {
			BundleID              string `json:"bundleId"`
			Environment           string `json:"environment"`
			SignedTransactionInfo string `json:"signedTransactionInfo"`
		} `json:"data"`
	}
	if err := a.jws.verify(signedPayload, &p); err != nil || p.NotificationUUID == "" {
		return AppleNotification{}, errBadSignature
	}
	n := AppleNotification{UUID: p.NotificationUUID, Type: p.NotificationType}
	if p.Subtype != "" {
		n.Type += "." + p.Subtype
	}
	if p.Data.SignedTransactionInfo != "" {
		var tx appleTxnPayload
		if err := a.jws.verify(p.Data.SignedTransactionInfo, &tx); err != nil {
			return AppleNotification{}, errBadSignature
		}
		n.TransactionID, n.OriginalTransactionID = tx.TransactionID, tx.OriginalTransactionID
	}
	s, err := a.creds(ctx)
	if err != nil {
		return AppleNotification{}, err
	}
	n.Ours = p.Data.BundleID == s.BundleID && p.Data.Environment == s.Environment
	return n, nil
}

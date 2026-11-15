package subscription

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"athlix/backend/internal/platform/secrets"
)

// ---- Apple-style certificate chain ----

type testChain struct {
	x5c     []string
	leafKey *ecdsa.PrivateKey
	rootFP  []byte
}

type certOpts struct {
	noLeafMarker bool
	leafExpired  bool
}

func newChain(t *testing.T, o certOpts) testChain {
	t.Helper()
	key := func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	marker := func(oid []int) []pkix.Extension {
		return []pkix.Extension{{Id: oid, Value: []byte{0x05, 0x00}}}
	}
	nb, na := fixedNow.Add(-24*time.Hour), fixedNow.Add(365*24*time.Hour)
	rootKey, interKey, leafKey := key(), key(), key()
	rootTpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test Root"},
		NotBefore: nb, NotAfter: na, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTpl, rootTpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := x509.ParseCertificate(rootDER)
	interTpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Test Intermediate"},
		NotBefore: nb, NotAfter: na, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
		ExtraExtensions: marker(oidAppleIntermediate)}
	interDER, err := x509.CreateCertificate(rand.Reader, interTpl, root, &interKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	inter, _ := x509.ParseCertificate(interDER)
	leafTpl := &x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "Test Leaf"},
		NotBefore: nb, NotAfter: na, KeyUsage: x509.KeyUsageDigitalSignature}
	if !o.noLeafMarker {
		leafTpl.ExtraExtensions = marker(oidAppleLeaf)
	}
	if o.leafExpired {
		leafTpl.NotAfter = fixedNow.Add(-time.Hour)
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTpl, inter, &leafKey.PublicKey, interKey)
	if err != nil {
		t.Fatal(err)
	}
	fp := sha256.Sum256(rootDER)
	enc := base64.StdEncoding.EncodeToString
	return testChain{x5c: []string{enc(leafDER), enc(interDER), enc(rootDER)}, leafKey: leafKey, rootFP: fp[:]}
}

func (c testChain) sign(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["x5c"] = c.x5c
	s, err := tok.SignedString(c.leafKey)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (c testChain) verifier() *jwsVerifier {
	return &jwsVerifier{rootSHA256: c.rootFP, now: func() time.Time { return fixedNow }}
}

func TestJWSVerifierAcceptsPinnedChain(t *testing.T) {
	c := newChain(t, certOpts{})
	var out struct {
		ID string `json:"notificationUUID"`
	}
	if err := c.verifier().verify(c.sign(t, jwt.MapClaims{"notificationUUID": "n-1"}), &out); err != nil || out.ID != "n-1" {
		t.Fatalf("err %v out %+v", err, out)
	}
}

func TestJWSVerifierRejects(t *testing.T) {
	c := newChain(t, certOpts{})
	good := c.sign(t, jwt.MapClaims{"a": 1})
	var out map[string]any

	// The production verifier pins Apple's real root, so a self-made chain fails.
	if err := newAppleJWSVerifier(func() time.Time { return fixedNow }).verify(good, &out); !errors.Is(err, errBadSignature) {
		t.Fatalf("real pin: %v", err)
	}
	// Signed by a key that is not the leaf's.
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"a": 1})
	tok.Header["x5c"] = c.x5c
	forged, _ := tok.SignedString(other)
	// Tampered payload.
	parts := strings.Split(good, ".")
	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"a":2}`)) + "." + parts[2]
	// Wrong algorithm.
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"a": 1})
	hs.Header["x5c"] = c.x5c
	hsSigned, _ := hs.SignedString([]byte("k"))
	// Only two certificates.
	short := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"a": 1})
	short.Header["x5c"] = c.x5c[:2]
	shortSigned, _ := short.SignedString(c.leafKey)

	noMarker := newChain(t, certOpts{noLeafMarker: true})
	expired := newChain(t, certOpts{leafExpired: true})
	cases := map[string]struct {
		v *jwsVerifier
		s string
	}{
		"forged":         {c.verifier(), forged},
		"tampered":       {c.verifier(), tampered},
		"hs256":          {c.verifier(), hsSigned},
		"short chain":    {c.verifier(), shortSigned},
		"garbage":        {c.verifier(), "a.b.c"},
		"no leaf marker": {noMarker.verifier(), noMarker.sign(t, jwt.MapClaims{"a": 1})},
		"expired leaf":   {expired.verifier(), expired.sign(t, jwt.MapClaims{"a": 1})},
		"other root":     {noMarker.verifier(), good},
	}
	for name, tc := range cases {
		if err := tc.v.verify(tc.s, &out); !errors.Is(err, errBadSignature) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// ---- App Store client ----

func ecPEM(t *testing.T) string {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func appleSecret(t *testing.T) secrets.Static {
	return secrets.Static{"as": map[string]any{"bundleId": "app.athlix", "issuerId": "iss", "keyId": "KID1",
		"privateKey": ecPEM(t), "environment": "Sandbox"}}
}

func TestAppleClientStatuses(t *testing.T) {
	c := newChain(t, certOpts{})
	exp := fixedNow.Add(24 * time.Hour).UnixMilli()
	grace := fixedNow.Add(72 * time.Hour).UnixMilli()
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/inApps/v1/subscriptions/1001" {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"lastTransactions": []any{
			map[string]any{"originalTransactionId": "1000", "status": 4,
				"signedTransactionInfo": c.sign(t, jwt.MapClaims{"transactionId": "1001", "originalTransactionId": "1000",
					"bundleId": "app.athlix", "productId": "plus_ios", "originalPurchaseDate": fixedNow.UnixMilli(),
					"expiresDate": exp, "appAccountToken": "abc"}),
				"signedRenewalInfo": c.sign(t, jwt.MapClaims{"autoRenewStatus": 1, "gracePeriodExpiresDate": grace})},
			map[string]any{"originalTransactionId": "2000", "status": 1,
				"signedTransactionInfo": c.sign(t, jwt.MapClaims{"bundleId": "com.other", "productId": "x"})},
		}}}})
	}))
	defer srv.Close()
	a := &appleClient{secrets: appleSecret(t), name: "as", http: srv.Client(), bases: map[string]string{"Sandbox": srv.URL},
		jws: c.verifier(), now: func() time.Time { return fixedNow }}
	subs, err := a.Statuses(context.Background(), "1001")
	if err != nil || len(subs) != 1 {
		t.Fatalf("err %v subs %+v", err, subs)
	}
	s := subs[0]
	if s.OriginalTransactionID != "1000" || s.ProductID != "plus_ios" || s.Status != 4 || !s.AutoRenew ||
		s.GraceExpiresAt == nil || s.GraceExpiresAt.UnixMilli() != grace || s.ExpiresAt.UnixMilli() != exp || s.AppAccountToken != "abc" {
		t.Fatalf("sub %+v", s)
	}
	// The API token is ES256 with our key id, audience and bundle id.
	tok, _, err := jwt.NewParser().ParseUnverified(strings.TrimPrefix(auth, "Bearer "), jwt.MapClaims{})
	if err != nil || tok.Method.Alg() != "ES256" || tok.Header["kid"] != "KID1" {
		t.Fatalf("api token %v %v", err, tok)
	}
	cl := tok.Claims.(jwt.MapClaims)
	if cl["aud"] != "appstoreconnect-v1" || cl["bid"] != "app.athlix" || cl["iss"] != "iss" {
		t.Fatalf("claims %v", cl)
	}
	if _, err := a.Statuses(context.Background(), "999"); !errors.Is(err, errNotFound) {
		t.Fatalf("missing txn: %v", err)
	}
	// A response signed by an unpinned chain is never trusted.
	a.jws = newAppleJWSVerifier(func() time.Time { return fixedNow })
	if _, err := a.Statuses(context.Background(), "1001"); !errors.Is(err, errUpstream) {
		t.Fatalf("unpinned: %v", err)
	}
	a.secrets = secrets.Static{}
	if _, err := a.Statuses(context.Background(), "1001"); !errors.Is(err, errStoreDisabled) {
		t.Fatalf("no secret: %v", err)
	}
}

func TestAppleClientVerifyNotification(t *testing.T) {
	c := newChain(t, certOpts{})
	a := &appleClient{secrets: appleSecret(t), name: "as", bases: map[string]string{"Sandbox": "http://unused"},
		jws: c.verifier(), now: func() time.Time { return fixedNow }}
	payload := func(bundle, env string) string {
		return c.sign(t, jwt.MapClaims{"notificationType": "DID_RENEW", "subtype": "BILLING_RECOVERY", "notificationUUID": "u-1",
			"data": map[string]any{"bundleId": bundle, "environment": env,
				"signedTransactionInfo": c.sign(t, jwt.MapClaims{"transactionId": "1001", "originalTransactionId": "1000"})}})
	}
	n, err := a.VerifyNotification(context.Background(), payload("app.athlix", "Sandbox"))
	if err != nil || !n.Ours || n.UUID != "u-1" || n.Type != "DID_RENEW.BILLING_RECOVERY" || n.OriginalTransactionID != "1000" || n.TransactionID != "1001" {
		t.Fatalf("err %v n %+v", err, n)
	}
	if n, _ := a.VerifyNotification(context.Background(), payload("app.athlix", "Production")); n.Ours {
		t.Fatal("other environment must not be ours")
	}
	if n, _ := a.VerifyNotification(context.Background(), payload("com.other", "Sandbox")); n.Ours {
		t.Fatal("other bundle must not be ours")
	}
	if _, err := a.VerifyNotification(context.Background(), c.sign(t, jwt.MapClaims{"notificationType": "TEST"})); !errors.Is(err, errBadSignature) {
		t.Fatalf("missing uuid: %v", err)
	}
}

// ---- Google Pub/Sub push verifier ----

type jwksServer struct {
	*httptest.Server
	hits atomic.Int32
	key  *rsa.PrivateKey
}

func newJWKS(t *testing.T) *jwksServer {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s := &jwksServer{key: k}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kid": "k1", "kty": "RSA",
			"n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes())}}})
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *jwksServer) token(t *testing.T, kid string, claims jwt.MapClaims) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(s.key)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + signed
}

func pushClaims() jwt.MapClaims {
	return jwt.MapClaims{"iss": "https://accounts.google.com", "aud": "https://api.athlix.test/v1/webhooks/google-play",
		"email": "push@proj.iam.gserviceaccount.com", "email_verified": true,
		"iat": fixedNow.Add(-time.Minute).Unix(), "exp": fixedNow.Add(time.Hour).Unix()}
}

func TestPubsubVerifier(t *testing.T) {
	srv := newJWKS(t)
	now := fixedNow
	p := &pubsubVerifier{name: "gp", http: srv.Client(), certsURL: srv.URL, now: func() time.Time { return now },
		secrets: secrets.Static{"gp": map[string]any{"pubsubAudience": "https://api.athlix.test/v1/webhooks/google-play",
			"pubsubServiceAccountEmail": "push@proj.iam.gserviceaccount.com"}}}
	ctx := context.Background()
	if err := p.Verify(ctx, srv.token(t, "k1", pushClaims())); err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if err := p.Verify(ctx, srv.token(t, "k1", pushClaims())); err != nil || srv.hits.Load() != 1 {
		t.Fatalf("keys must be cached: %v hits %d", err, srv.hits.Load())
	}
	mutate := func(k string, v any) string {
		c := pushClaims()
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
		return srv.token(t, "k1", c)
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	forgedTok := jwt.NewWithClaims(jwt.SigningMethodRS256, pushClaims())
	forgedTok.Header["kid"] = "k1"
	forged, _ := forgedTok.SignedString(other)
	for name, h := range map[string]string{
		"no bearer":      "Basic abc",
		"empty":          "",
		"wrong audience": mutate("aud", "https://evil.test"),
		"wrong email":    mutate("email", "x@evil.test"),
		"unverified":     mutate("email_verified", false),
		"wrong issuer":   mutate("iss", "https://evil.test"),
		"expired":        mutate("exp", fixedNow.Add(-time.Hour).Unix()),
		"no expiry":      mutate("exp", nil),
		"forged":         "Bearer " + forged,
	} {
		if err := p.Verify(ctx, h); !errors.Is(err, errBadSignature) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// An unknown kid triggers at most one refetch per minute.
	before := srv.hits.Load()
	for i := 0; i < 3; i++ {
		_ = p.Verify(ctx, srv.token(t, "k2", pushClaims()))
	}
	if srv.hits.Load() > before+1 {
		t.Fatalf("refetch not rate limited: %d", srv.hits.Load()-before)
	}
	p.secrets = secrets.Static{}
	if err := p.Verify(ctx, srv.token(t, "k1", pushClaims())); !errors.Is(err, errStoreDisabled) {
		t.Fatalf("no secret: %v", err)
	}
}

// ---- Google Play client ----

func rsaPEM(t *testing.T) string {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestPlayClient(t *testing.T) {
	var tokenCalls, acks atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			tokenCalls.Add(1)
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || r.Form.Get("assertion") == "" {
				w.WriteHeader(400)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"at-1","expires_in":3600}`))
		case r.Header.Get("Authorization") != "Bearer at-1":
			w.WriteHeader(401)
		case strings.HasSuffix(r.URL.Path, "/purchases/subscriptionsv2/tokens/good"):
			_, _ = w.Write([]byte(`{"startTime":"2026-11-01T00:00:00Z","subscriptionState":"SUBSCRIPTION_STATE_ACTIVE",
				"acknowledgementState":"ACKNOWLEDGEMENT_STATE_PENDING","linkedPurchaseToken":"old",
				"externalAccountIdentifiers":{"obfuscatedExternalAccountId":"acct"},
				"lineItems":[{"productId":"a","expiryTime":"2026-11-20T00:00:00Z"},
					{"productId":"plus_android","expiryTime":"2026-12-01T00:00:00.5Z","autoRenewingPlan":{"autoRenewEnabled":true}}]}`))
		case strings.HasSuffix(r.URL.Path, "/tokens/gone"):
			w.WriteHeader(410)
		case strings.HasSuffix(r.URL.Path, "/purchases/subscriptions/plus_android/tokens/good:acknowledge") && r.Method == http.MethodPost:
			acks.Add(1)
		default:
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	p := &playClient{name: "gp", http: srv.Client(), tokenURL: srv.URL + "/token", apiBase: srv.URL, now: func() time.Time { return fixedNow },
		secrets: secrets.Static{"gp": map[string]any{"packageName": "app.athlix", "clientEmail": "sa@proj.iam.gserviceaccount.com", "privateKey": rsaPEM(t)}}}
	ctx := context.Background()
	s, err := p.Subscription(ctx, "good")
	if err != nil {
		t.Fatal(err)
	}
	if s.ProductID != "plus_android" || !s.AutoRenew || s.Acknowledged || s.ObfuscatedAccountID != "acct" || s.LinkedPurchaseToken != "old" ||
		!s.ExpiresAt.Equal(time.Date(2026, 12, 1, 0, 0, 0, 5e8, time.UTC)) || !s.StartedAt.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("sub %+v", s)
	}
	if err := p.Acknowledge(ctx, "plus_android", "good"); err != nil || acks.Load() != 1 {
		t.Fatalf("ack %v %d", err, acks.Load())
	}
	if _, err := p.Subscription(ctx, "gone"); !errors.Is(err, errNotFound) {
		t.Fatalf("gone: %v", err)
	}
	if _, err := p.Subscription(ctx, "boom"); !errors.Is(err, errUpstream) || strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("boom: %v", err)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("access token must be cached, got %d calls", tokenCalls.Load())
	}
	p.secrets = secrets.Static{}
	if _, err := p.Subscription(ctx, "good"); !errors.Is(err, errStoreDisabled) {
		t.Fatalf("no secret: %v", err)
	}
}

package subscription

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"athlix/backend/internal/platform/secrets"
)

// appleRootG3SHA256 pins "Apple Root CA - G3" (https://www.apple.com/certificateauthority/) by the
// SHA-256 of its DER encoding. The x5c chain in every App Store JWS must end in this root.
const appleRootG3SHA256 = "63343abfb89a6a03ebb57e9b3f5fa7be7c4f5c756f3017b3a8c488c3653e9179"

// Marker extensions Apple puts on its App Store signing certificates.
var (
	oidAppleLeaf         = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1}
	oidAppleIntermediate = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1}
)

// jwsVerifier checks Apple-signed JWS values (signedPayload, signedTransactionInfo,
// signedRenewalInfo): ES256, x5c = [leaf, intermediate, root], root pinned by fingerprint.
type jwsVerifier struct {
	rootSHA256 []byte // pinned root fingerprint; tests inject their own
	now        func() time.Time
}

func newAppleJWSVerifier(now func() time.Time) *jwsVerifier {
	fp, _ := hex.DecodeString(appleRootG3SHA256)
	return &jwsVerifier{rootSHA256: fp, now: now}
}

// verify checks signed and decodes its payload into out. Any failure is errBadSignature.
func (v *jwsVerifier) verify(signed string, out any) error {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"ES256"}), jwt.WithoutClaimsValidation())
	if _, err := parser.Parse(signed, v.leafKey); err != nil {
		return errBadSignature
	}
	parts := strings.Split(signed, ".")
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(b, out) != nil {
		return errBadSignature
	}
	return nil
}

func (v *jwsVerifier) leafKey(t *jwt.Token) (any, error) {
	raw, ok := t.Header["x5c"].([]any)
	if !ok || len(raw) != 3 {
		return nil, errBadSignature
	}
	certs := make([]*x509.Certificate, 3)
	for i, r := range raw {
		s, ok := r.(string)
		if !ok {
			return nil, errBadSignature
		}
		der, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, errBadSignature
		}
		if certs[i], err = x509.ParseCertificate(der); err != nil {
			return nil, errBadSignature
		}
	}
	leaf, inter, root := certs[0], certs[1], certs[2]
	fp := sha256.Sum256(root.Raw)
	if len(v.rootSHA256) != sha256.Size || !bytes.Equal(fp[:], v.rootSHA256) {
		return nil, errBadSignature
	}
	if !hasExt(leaf, oidAppleLeaf) || !hasExt(inter, oidAppleIntermediate) {
		return nil, errBadSignature
	}
	roots, inters := x509.NewCertPool(), x509.NewCertPool()
	roots.AddCert(root)
	inters.AddCert(inter)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: inters, CurrentTime: v.now(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		return nil, errBadSignature
	}
	pk, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errBadSignature
	}
	return pk, nil
}

func hasExt(c *x509.Certificate, oid asn1.ObjectIdentifier) bool {
	for _, e := range c.Extensions {
		if e.Id.Equal(oid) {
			return true
		}
	}
	return false
}

// ---- Google Pub/Sub push OIDC ----

const googleCertsURL = "https://www.googleapis.com/oauth2/v3/certs"

// jwksTTL is how long fetched Google signing keys are trusted before a refresh;
// jwksMinRefetch stops an unknown kid from triggering a fetch on every request.
const (
	jwksTTL        = time.Hour
	jwksMinRefetch = time.Minute
)

// pubsubVerifier verifies the `Authorization: Bearer` OIDC token Pub/Sub sends with RTDN pushes.
type pubsubVerifier struct {
	secrets  secrets.Getter
	name     string
	http     *http.Client
	certsURL string
	now      func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

type pubsubClaims struct {
	jwt.RegisteredClaims
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

// Verify returns nil only for a valid token from the configured push service account.
func (p *pubsubVerifier) Verify(ctx context.Context, authorization string) error {
	tok, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok || tok == "" {
		return errBadSignature
	}
	var s googlePlaySecret
	if err := p.secrets.GetJSON(ctx, p.name, &s); err != nil || s.PubsubAudience == "" || s.PubsubServiceAccountEmail == "" {
		return errStoreDisabled
	}
	claims := &pubsubClaims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}), jwt.WithAudience(s.PubsubAudience),
		jwt.WithExpirationRequired(), jwt.WithTimeFunc(p.now), jwt.WithLeeway(30*time.Second))
	if _, err := parser.ParseWithClaims(tok, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return p.key(ctx, kid)
	}); err != nil {
		return errBadSignature
	}
	if claims.Issuer != "accounts.google.com" && claims.Issuer != "https://accounts.google.com" {
		return errBadSignature
	}
	if !claims.EmailVerified || claims.Email != s.PubsubServiceAccountEmail {
		return errBadSignature
	}
	return nil
}

func (p *pubsubVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	k, ok := p.keys[kid]
	stale := now.Sub(p.fetched) > jwksTTL
	if ok && !stale {
		return k, nil
	}
	if !stale && now.Sub(p.fetched) < jwksMinRefetch {
		return nil, errBadSignature
	}
	keys, err := fetchJWKS(ctx, p.http, p.certsURL)
	if err != nil {
		if ok {
			return k, nil // keep serving the cached key if Google is briefly unreachable
		}
		return nil, err
	}
	p.keys, p.fetched = keys, now
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, errBadSignature
}

func fetchJWKS(ctx context.Context, hc *http.Client, url string) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errUpstream
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, errUpstream
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errUpstream
	}
	var doc struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&doc); err != nil {
		return nil, errUpstream
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		out[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(out) == 0 {
		return nil, errors.New("jwks: no usable keys")
	}
	return out, nil
}

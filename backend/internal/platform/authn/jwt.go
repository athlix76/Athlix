// Package authn issues/verifies access tokens and provides the auth middleware that EVERY
// route group uses (see internal/app/router.go). Refresh tokens, OTP and admin login live
// in modules/auth and modules/admin; this package only handles stateless access tokens.
package authn

import (
	"encoding/base64"
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// SigningKeys is the JSON shape of the "jwt" secret. User and admin tokens use different keys
// so a user token can never be accepted as an admin token.
type SigningKeys struct {
	UserSigningKey  string `json:"userSigningKey"`
	AdminSigningKey string `json:"adminSigningKey"`
}

// Claims is the access-token payload.
type Claims struct {
	Type        string   `json:"typ"` // "user" | "admin"
	RoleID      int64    `json:"rid,omitempty"`
	Permissions []string `json:"perms,omitempty"`
	jwt.RegisteredClaims
}

// SubjectID returns the numeric subject.
func (c *Claims) SubjectID() int64 {
	id, _ := strconv.ParseInt(c.Subject, 10, 64)
	return id
}

// Issuer signs and verifies HS256 tokens.
type Issuer struct {
	userKey, adminKey []byte
	issuer            string
	now               func() time.Time
}

// ErrInvalidToken is returned for any bad/expired token.
var ErrInvalidToken = errors.New("authn: invalid token")

// NewIssuer decodes keys (each >= 32 bytes, base64).
func NewIssuer(k SigningKeys, issuer string) (*Issuer, error) {
	uk, err1 := base64.StdEncoding.DecodeString(k.UserSigningKey)
	ak, err2 := base64.StdEncoding.DecodeString(k.AdminSigningKey)
	if err1 != nil || err2 != nil || len(uk) < 32 || len(ak) < 32 {
		return nil, errors.New("authn: signing keys must be >= 32 bytes base64")
	}
	return &Issuer{userKey: uk, adminKey: ak, issuer: issuer, now: time.Now}, nil
}

// IssueUser returns an access token for an app user.
func (i *Issuer) IssueUser(userID int64, ttl time.Duration) (string, time.Time, error) {
	return i.sign(i.userKey, Claims{Type: "user"}, userID, ttl)
}

// IssueAdmin returns an access token for an admin with its permissions embedded.
func (i *Issuer) IssueAdmin(adminID, roleID int64, perms []string, ttl time.Duration) (string, time.Time, error) {
	return i.sign(i.adminKey, Claims{Type: "admin", RoleID: roleID, Permissions: perms}, adminID, ttl)
}

func (i *Issuer) sign(key []byte, c Claims, sub int64, ttl time.Duration) (string, time.Time, error) {
	now := i.now()
	exp := now.Add(ttl)
	c.RegisteredClaims = jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(sub, 10),
		Issuer:    i.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
		ID:        uuid.NewString(),
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(key)
	return s, exp, err
}

// ParseUser verifies a user access token.
func (i *Issuer) ParseUser(tok string) (*Claims, error) { return i.parse(tok, i.userKey, "user") }

// ParseAdmin verifies an admin access token.
func (i *Issuer) ParseAdmin(tok string) (*Claims, error) { return i.parse(tok, i.adminKey, "admin") }

func (i *Issuer) parse(tok string, key []byte, typ string) (*Claims, error) {
	c := &Claims{}
	t, err := jwt.ParseWithClaims(tok, c, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return key, nil
	}, jwt.WithIssuer(i.issuer), jwt.WithExpirationRequired(), jwt.WithTimeFunc(i.now))
	if err != nil || !t.Valid || c.Type != typ || c.SubjectID() <= 0 {
		return nil, ErrInvalidToken
	}
	return c, nil
}

// Package crypto encrypts sensitive PII at rest and builds keyed lookup hashes.
//
//   - Encryption: AES-256-GCM, random 12-byte nonce per value, key versioned so keys can rotate.
//     Ciphertext layout: [2-byte big-endian key version][12-byte nonce][ciphertext+tag].
//   - Lookup: HMAC-SHA256 with a separate secret key, scoped by a purpose string so the same
//     value hashed for two purposes gives different hashes.
//
// Keys come ONLY from the "pii-keys" secret in AWS Secrets Manager (see docs/security.md).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
)

// Keys is the JSON shape of the "pii-keys" secret.
//
//	{"current": 1, "keys": {"1": "<base64 32 bytes>"}, "hmacKey": "<base64 >=32 bytes>"}
type Keys struct {
	Current int               `json:"current"`
	Keys    map[string]string `json:"keys"`
	HMACKey string            `json:"hmacKey"`
}

// Box encrypts/decrypts PII. Safe for concurrent use.
type Box struct {
	current uint16
	aeads   map[uint16]cipher.AEAD
	hmacKey []byte
}

// ErrDecrypt is returned for tampered data or unknown key versions.
var ErrDecrypt = errors.New("crypto: cannot decrypt value")

// NewBox validates the keys (each exactly 32 random bytes) and builds a Box.
func NewBox(k Keys) (*Box, error) {
	b := &Box{aeads: map[uint16]cipher.AEAD{}}
	for vs, enc := range k.Keys {
		v, err := strconv.Atoi(vs)
		if err != nil || v <= 0 || v > 65535 {
			return nil, fmt.Errorf("crypto: bad key version %q", vs)
		}
		raw, err := base64.StdEncoding.DecodeString(enc)
		if err != nil || len(raw) != 32 {
			return nil, fmt.Errorf("crypto: key version %d must be 32 bytes base64", v)
		}
		block, err := aes.NewCipher(raw)
		if err != nil {
			return nil, err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		b.aeads[uint16(v)] = aead
	}
	b.current = uint16(k.Current)
	if _, ok := b.aeads[b.current]; !ok {
		return nil, fmt.Errorf("crypto: current key version %d missing", k.Current)
	}
	hk, err := base64.StdEncoding.DecodeString(k.HMACKey)
	if err != nil || len(hk) < 32 {
		return nil, errors.New("crypto: hmacKey must be at least 32 bytes base64")
	}
	b.hmacKey = hk
	return b, nil
}

// KeyVersion is the version new values are encrypted with (store it next to _enc columns).
func (b *Box) KeyVersion() uint16 { return b.current }

// Encrypt seals plaintext with the current key.
func (b *Box) Encrypt(plain []byte) ([]byte, error) {
	aead := b.aeads[b.current]
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 2, 2+len(nonce)+len(plain)+aead.Overhead())
	binary.BigEndian.PutUint16(out, b.current)
	out = append(out, nonce...)
	return aead.Seal(out, nonce, plain, nil), nil
}

// EncryptString is Encrypt for strings.
func (b *Box) EncryptString(s string) ([]byte, error) { return b.Encrypt([]byte(s)) }

// Decrypt opens a value sealed by any known key version.
func (b *Box) Decrypt(data []byte) ([]byte, error) {
	if len(data) < 2 {
		return nil, ErrDecrypt
	}
	aead, ok := b.aeads[binary.BigEndian.Uint16(data)]
	if !ok {
		return nil, ErrDecrypt
	}
	rest := data[2:]
	if len(rest) < aead.NonceSize()+aead.Overhead() {
		return nil, ErrDecrypt
	}
	plain, err := aead.Open(nil, rest[:aead.NonceSize()], rest[aead.NonceSize():], nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}

// DecryptString is Decrypt for strings.
func (b *Box) DecryptString(data []byte) (string, error) {
	p, err := b.Decrypt(data)
	return string(p), err
}

// Hash returns HMAC-SHA256(purpose || 0x00 || value) for exact-match lookups
// (e.g. Hash("mobile", "+919876543210")). Store it in BINARY(32) columns.
func (b *Box) Hash(purpose, value string) []byte {
	m := hmac.New(sha256.New, b.hmacKey)
	m.Write([]byte(purpose))
	m.Write([]byte{0})
	m.Write([]byte(value))
	return m.Sum(nil)
}

// SHA256 returns a plain SHA-256 (for random high-entropy tokens like refresh tokens).
func SHA256(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

// RandomToken returns n random bytes as URL-safe base64 (no padding).
func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Last4 returns the last 4 characters of s (or s if shorter) for masked display.
func Last4(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}

// MaskMobile renders "******1234".
func MaskMobile(last4 string) string { return "******" + last4 }

// StoreAccountToken is the stable per-user UUID passed to Google Play (obfuscatedAccountId)
// and the App Store (appAccountToken): the first 16 bytes of Hash("store_account", userID)
// with the UUID version-4 and RFC 4122 variant bits set. It is the ONE implementation; the
// user module returns it as Me.storeAccountToken and subscription checks purchases with it.
func (b *Box) StoreAccountToken(userID int64) string {
	h := b.Hash("store_account", strconv.FormatInt(userID, 10))
	var u [16]byte
	copy(u[:], h[:16])
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80
	x := hex.EncodeToString(u[:])
	return x[0:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:32]
}

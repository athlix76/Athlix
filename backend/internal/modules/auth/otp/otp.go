// Package otp holds the one-time-code helpers shared by login (auth) and parent consent
// (guardian): Indian mobile normalisation, code generation, and a Redis challenge store
// that keeps only the SHA-256 of the code and burns a challenge after too many wrong tries.
package otp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// NormalizeMobile accepts a 10 digit Indian mobile with an optional +91 / 91 / 0 prefix
// (spaces and dashes ignored) and returns it as +91XXXXXXXXXX.
func NormalizeMobile(s string) (string, bool) {
	s = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(s, "+91"):
		s = s[3:]
	case len(s) == 12 && strings.HasPrefix(s, "91"):
		s = s[2:]
	case len(s) == 11 && strings.HasPrefix(s, "0"):
		s = s[1:]
	}
	if len(s) != 10 || s[0] < '6' || s[0] > '9' {
		return "", false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return "+91" + s, true
}

// SMSNumber turns +91XXXXXXXXXX into the 91XXXXXXXXXX form the SMS sender expects.
func SMSNumber(e164 string) string { return strings.TrimPrefix(e164, "+") }

// Generate returns a random numeric code of the given length.
func Generate(length int) (string, error) {
	if length < 4 || length > 10 {
		length = 6
	}
	var b strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + n.Int64()))
	}
	return b.String(), nil
}

// Hash returns hex(SHA-256(scope ":" code)). The scope (request id / consent id) stops a
// stolen hash from matching another challenge.
func Hash(scope, code string) string {
	h := sha256.Sum256([]byte(scope + ":" + strings.TrimSpace(code)))
	return hex.EncodeToString(h[:])
}

// Result of checking a code.
type Result int

const (
	// Missing: no such challenge (expired, used, or burnt).
	Missing Result = iota
	// OK: the code matched; the challenge is now deleted (single use).
	OK
	// Wrong: the code did not match; tries remain.
	Wrong
	// Burnt: the code did not match and the attempt limit was reached; challenge deleted.
	Burnt
)

const (
	fieldHash     = "h"
	fieldAttempts = "a"
)

// Store keeps OTP challenges in Redis hashes.
type Store struct{ rdb *redis.Client }

// NewStore builds a Store.
func NewStore(rdb *redis.Client) *Store { return &Store{rdb: rdb} }

// Save stores a challenge under key with the hashed code plus extra fields (never raw PII).
func (s *Store) Save(ctx context.Context, key, codeHash string, extra map[string]string, ttl time.Duration) error {
	vals := map[string]any{fieldHash: codeHash, fieldAttempts: 0}
	for k, v := range extra {
		vals[k] = v
	}
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, key)
	pipe.HSet(ctx, key, vals)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// Check compares code against the challenge. On OK it returns the extra fields.
func (s *Store) Check(ctx context.Context, key, scope, code string, maxAttempts int) (Result, map[string]string, error) {
	vals, err := s.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return Missing, nil, err
	}
	stored, ok := vals[fieldHash]
	if !ok {
		return Missing, nil, nil
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(Hash(scope, code))) == 1 {
		n, err := s.rdb.Del(ctx, key).Result()
		if err != nil {
			return Missing, nil, err
		}
		if n == 0 { // a concurrent request used it first
			return Missing, nil, nil
		}
		delete(vals, fieldHash)
		delete(vals, fieldAttempts)
		return OK, vals, nil
	}
	n, err := s.rdb.HIncrBy(ctx, key, fieldAttempts, 1).Result()
	if err != nil {
		return Missing, nil, err
	}
	if maxAttempts <= 0 || n >= int64(maxAttempts) {
		if err := s.rdb.Del(ctx, key).Err(); err != nil {
			return Missing, nil, err
		}
		return Burnt, nil, nil
	}
	return Wrong, nil, nil
}

// Cooldown sets key for ttl if absent. It returns false when the key already exists
// (the caller must wait). ttl <= 0 disables the cooldown.
func (s *Store) Cooldown(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		return true, nil
	}
	return s.rdb.SetNX(ctx, key, "1", ttl).Result()
}

// ErrNotNumeric is returned by ParseInt64 for bad ids stored in a challenge.
var ErrNotNumeric = errors.New("otp: field is not numeric")

// ParseInt64 reads a numeric field saved with Save.
func ParseInt64(fields map[string]string, k string) (int64, error) {
	v, err := strconv.ParseInt(fields[k], 10, 64)
	if err != nil {
		return 0, ErrNotNumeric
	}
	return v, nil
}

// Package ratelimit is a Redis fixed-window limiter.
package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter counts hits per key per window.
type Limiter struct{ rdb *redis.Client }

// New returns a Limiter.
func New(rdb *redis.Client) *Limiter { return &Limiter{rdb: rdb} }

// Allow increments key and reports whether the count is still <= limit within window.
// Keys must not contain raw PII: hash mobiles first (crypto.Box.Hash) and hex-encode.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	k := "rl:" + key
	pipe := l.rdb.TxPipeline()
	incr := pipe.Incr(ctx, k)
	pipe.ExpireNX(ctx, k, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return incr.Val() <= int64(limit), nil
}

// Package cache creates the Redis client. REDIS_ADDR=miniredis starts an in-process Redis
// for local development and tests (never allowed outside ENV=local).
//
// Redis holds only short-lived, non-secret state: OTP request records (hashed OTP),
// rate-limit counters, WebSocket connection ids, ws tickets, DigiLocker OAuth state.
package cache

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// Options configure the client. TLS is on outside local (ElastiCache in-transit encryption,
// config REDIS_TLS). Password is an optional AUTH token read from Secrets Manager by the
// caller; it is never taken from an environment variable or Terraform.
type Options struct {
	Addr     string
	TLS      bool
	Password string
}

// New connects to o.Addr, or starts miniredis when o.Addr == "miniredis".
func New(o Options) (*redis.Client, func(), error) {
	addr := o.Addr
	if addr == "miniredis" {
		mr, err := miniredis.Run()
		if err != nil {
			return nil, nil, err
		}
		stop := followWallClock(mr, 250*time.Millisecond)
		c := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		return c, func() { stop(); _ = c.Close(); mr.Close() }, nil
	}
	ro := &redis.Options{Addr: addr, Password: o.Password, DialTimeout: 2 * time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second}
	if o.TLS {
		ro.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	c := redis.NewClient(ro)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, nil, fmt.Errorf("cache: cannot reach redis")
	}
	return c, func() { _ = c.Close() }, nil
}

// followWallClock makes miniredis expire keys in real time. miniredis only advances its
// TTL clock on FastForward, so without this an OTP cooldown or rate-limit window set by a
// local API run would never end. Tests use their own miniredis and keep full control.
func followWallClock(mr *miniredis.Miniredis, every time.Duration) (stop func()) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		last := time.Now()
		for {
			select {
			case <-done:
				return
			case now := <-t.C:
				mr.FastForward(now.Sub(last))
				last = now
			}
		}
	}()
	return func() { close(done) }
}

package cache

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// The local in-process Redis must expire keys in real time, like a real Redis: otherwise an
// OTP resend cooldown or a rate-limit window set during a local run never ends.
func TestMiniredisExpiresKeysInRealTime(t *testing.T) {
	c, closeFn, err := New(Options{Addr: "miniredis"})
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	ctx := context.Background()
	if err := c.Set(ctx, "cooldown", "1", 500*time.Millisecond).Err(); err != nil {
		t.Fatal(err)
	}
	if v, err := c.Get(ctx, "cooldown").Result(); err != nil || v != "1" {
		t.Fatalf("key should exist right after SET: %q %v", v, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := c.Get(ctx, "cooldown").Result(); err == redis.Nil {
			return // expired, as with a real Redis
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("key with a 500ms TTL still exists after 3s: miniredis is not following the wall clock")
}

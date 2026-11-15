package otp

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestNormalizeMobile(t *testing.T) {
	ok := map[string]string{
		"9876543210":      "+919876543210",
		"+91 98765-43210": "+919876543210",
		"919876543210":    "+919876543210",
		"09876543210":     "+919876543210",
		"6000000000":      "+916000000000",
	}
	for in, want := range ok {
		if got, valid := NormalizeMobile(in); !valid || got != want {
			t.Errorf("%q -> %q %v", in, got, valid)
		}
	}
	for _, in := range []string{"", "5876543210", "98765", "98765432101", "98765abcde", "+1 9876543210", "+9198765432"} {
		if _, valid := NormalizeMobile(in); valid {
			t.Errorf("%q should be invalid", in)
		}
	}
	if SMSNumber("+919876543210") != "919876543210" {
		t.Fatal("sms number")
	}
}

func TestGenerate(t *testing.T) {
	for _, n := range []int{4, 6, 8} {
		c, err := Generate(n)
		if err != nil || len(c) != n {
			t.Fatalf("len %d: %q %v", n, c, err)
		}
	}
	if c, _ := Generate(99); len(c) != 6 {
		t.Fatal("out of range length must fall back to 6")
	}
}

func store(t *testing.T) (*Store, *miniredis.Miniredis) {
	mr := miniredis.RunT(t)
	return NewStore(redis.NewClient(&redis.Options{Addr: mr.Addr()})), mr
}

func TestCheckFlow(t *testing.T) {
	ctx := context.Background()
	s, mr := store(t)
	if err := s.Save(ctx, "k", Hash("r1", "123456"), map[string]string{"uid": "42"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if mr.TTL("k") != time.Minute {
		t.Fatal("ttl not set")
	}
	if r, _, _ := s.Check(ctx, "k", "r1", "000000", 3); r != Wrong {
		t.Fatal("want wrong", r)
	}
	// the hash is scoped: the right code under another scope does not match
	if r, _, _ := s.Check(ctx, "k", "r2", "123456", 3); r != Wrong {
		t.Fatal("scope must matter", r)
	}
	r, extra, err := s.Check(ctx, "k", "r1", " 123456 ", 3)
	if err != nil || r != OK || extra["uid"] != "42" || len(extra) != 1 {
		t.Fatal(r, extra, err)
	}
	if id, err := ParseInt64(extra, "uid"); err != nil || id != 42 {
		t.Fatal(id, err)
	}
	if r, _, _ := s.Check(ctx, "k", "r1", "123456", 3); r != Missing {
		t.Fatal("code must be single use", r)
	}
}

func TestCheckBurnsAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)
	_ = s.Save(ctx, "k", Hash("r", "1111"), nil, time.Minute)
	if r, _, _ := s.Check(ctx, "k", "r", "2222", 2); r != Wrong {
		t.Fatal(r)
	}
	if r, _, _ := s.Check(ctx, "k", "r", "2222", 2); r != Burnt {
		t.Fatal(r)
	}
	if r, _, _ := s.Check(ctx, "k", "r", "1111", 2); r != Missing {
		t.Fatal("burnt challenge must be gone", r)
	}
	// re-saving resets attempts
	_ = s.Save(ctx, "k", Hash("r", "1111"), nil, time.Minute)
	if r, _, _ := s.Check(ctx, "k", "r", "1111", 2); r != OK {
		t.Fatal(r)
	}
}

func TestCooldown(t *testing.T) {
	ctx := context.Background()
	s, mr := store(t)
	if ok, _ := s.Cooldown(ctx, "c", 30*time.Second); !ok {
		t.Fatal("first call must pass")
	}
	if ok, _ := s.Cooldown(ctx, "c", 30*time.Second); ok {
		t.Fatal("second call must wait")
	}
	mr.FastForward(31 * time.Second)
	if ok, _ := s.Cooldown(ctx, "c", 30*time.Second); !ok {
		t.Fatal("cooldown must expire")
	}
	if ok, _ := s.Cooldown(ctx, "c", 0); !ok {
		t.Fatal("zero ttl disables the cooldown")
	}
	if _, err := ParseInt64(map[string]string{"x": "a"}, "x"); err != ErrNotNumeric {
		t.Fatal(err)
	}
}

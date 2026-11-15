package config

import "testing"

func TestLoadRules(t *testing.T) {
	t.Setenv("ENV", "dev")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("MEDIA_BUCKET", "")
	if _, err := Load(); err == nil {
		t.Fatal("full binaries need Redis and the media bucket outside local")
	}
	c, err := LoadDBOnly()
	if err != nil {
		t.Fatalf("migrate/admincli must load without Redis or media: %v", err)
	}
	if !c.RedisTLS {
		t.Fatal("Redis TLS must default on outside local")
	}
	t.Setenv("DB_DSN", "root@tcp(x)/y")
	if _, err := LoadDBOnly(); err == nil {
		t.Fatal("DB_DSN is never allowed outside local")
	}
}

func TestLocalDefaults(t *testing.T) {
	t.Setenv("ENV", "local")
	t.Setenv("REDIS_TLS", "")
	c, err := Load()
	if err != nil || c.RedisTLS || c.RedisAddr != "miniredis" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestDBHostOverrideOnlyForTools(t *testing.T) {
	t.Setenv("ENV", "dev")
	t.Setenv("REDIS_ADDR", "redis.internal:6379")
	t.Setenv("MEDIA_BUCKET", "b")
	t.Setenv("DB_HOST_OVERRIDE", "127.0.0.1:13306")
	if c, _ := Load(); c.DBHostOverride != "" {
		t.Fatal("app binaries must ignore DB_HOST_OVERRIDE")
	}
	if c, _ := LoadDBOnly(); c.DBHostOverride != "127.0.0.1:13306" {
		t.Fatal("tools honour DB_HOST_OVERRIDE")
	}
}

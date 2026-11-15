// Package config loads the process bootstrap configuration from environment variables.
//
// Environment variables hold only NON-secret values and the NAMES of secrets.
// Secret values (DB password, signing keys, partner keys) come from AWS Secrets Manager
// via internal/platform/secrets. Business rules (fees, limits, switches) live in the
// app_config table and are read through internal/platform/settings.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Env is the deployment environment.
type Env string

const (
	EnvLocal Env = "local"
	EnvDev   Env = "dev"
	EnvProd  Env = "prod"
)

// Config is the bootstrap configuration shared by every entrypoint.
type Config struct {
	Env              Env
	AWSRegion        string
	SecretPrefix     string // e.g. "athlix/dev"; secret names are SecretPrefix + "/" + name
	DBDSN            string // local only; in AWS the DSN is built from the "db" secret
	RedisAddr        string // "miniredis" starts an in-process Redis (local/tests only)
	RedisTLS         bool   // REDIS_TLS; default true outside local (ElastiCache in-transit encryption)
	MediaBucket      string
	LocalStorageDir  string
	EventsQueueURL   string // empty => in-process event bus
	WSManagementURL  string // API Gateway WebSocket management endpoint (https://...)
	WSPublicURL      string // wss://... returned to clients with a ws ticket
	PublicAPIBaseURL string // e.g. https://api.athlix.in (used for OAuth callbacks)
	AppDeepLinkBase  string // e.g. athlix://
	AdminOrigins     []string
	HTTPAddr         string
	DevFixedOTP      string // honoured ONLY when Env == local
	DBHostOverride   string // DB_HOST_OVERRIDE host[:port]; LoadDBOnly tools only (SSH tunnel / bastion)
}

// Load reads Config from the environment and validates it for a full app binary
// (api, ws, worker): Redis and the media bucket are required outside local.
func Load() (*Config, error) { return load(true) }

// LoadDBOnly is Load for tools that only touch MySQL (cmd/migrate, cmd/admincli):
// REDIS_ADDR and MEDIA_BUCKET are not required.
func LoadDBOnly() (*Config, error) { return load(false) }

func load(full bool) (*Config, error) {
	c := &Config{
		Env:              Env(get("ENV", "local")),
		AWSRegion:        get("AWS_REGION", "ap-south-1"),
		SecretPrefix:     get("SECRET_PREFIX", "athlix/local"),
		DBDSN:            os.Getenv("DB_DSN"),
		RedisAddr:        get("REDIS_ADDR", "miniredis"),
		MediaBucket:      os.Getenv("MEDIA_BUCKET"),
		LocalStorageDir:  get("LOCAL_STORAGE_DIR", ".localstorage"),
		EventsQueueURL:   os.Getenv("EVENTS_QUEUE_URL"),
		WSManagementURL:  os.Getenv("WS_MANAGEMENT_URL"),
		WSPublicURL:      get("WS_PUBLIC_URL", "ws://localhost:8080/v1/chat/ws"),
		PublicAPIBaseURL: get("PUBLIC_API_BASE_URL", "http://localhost:8080"),
		AppDeepLinkBase:  get("APP_DEEP_LINK_BASE", "athlix://"),
		// Local admin panel port. Not 5000: macOS AirPlay Receiver holds that port.
		// AWS always sets ADMIN_ORIGINS explicitly (infra/modules/stack).
		AdminOrigins: splitCSV(get("ADMIN_ORIGINS", "http://localhost:5050")),
		HTTPAddr:     get("HTTP_ADDR", ":8080"),
		DevFixedOTP:  os.Getenv("DEV_FIXED_OTP"),
	}
	if !full {
		c.DBHostOverride = os.Getenv("DB_HOST_OVERRIDE")
	}
	c.RedisTLS = get("REDIS_TLS", map[bool]string{true: "false", false: "true"}[c.Env == EnvLocal]) == "true"
	if err := c.validate(full); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate(full bool) error {
	switch c.Env {
	case EnvLocal, EnvDev, EnvProd:
	default:
		return fmt.Errorf("config: ENV must be local, dev or prod, got %q", c.Env)
	}
	if c.Env != EnvLocal {
		if c.DBDSN != "" {
			return fmt.Errorf("config: DB_DSN is only allowed when ENV=local; use the db secret")
		}
		if full && c.RedisAddr == "miniredis" {
			return fmt.Errorf("config: REDIS_ADDR must point at ElastiCache outside local")
		}
		if full && c.MediaBucket == "" {
			return fmt.Errorf("config: MEDIA_BUCKET is required outside local")
		}
		c.DevFixedOTP = ""
	}
	return nil
}

// IsLocal reports whether this is a developer machine.
func (c *Config) IsLocal() bool { return c.Env == EnvLocal }

// IsProd reports whether this is production.
func (c *Config) IsProd() bool { return c.Env == EnvProd }

// SecretName returns the full Secrets Manager name for a short name like "jwt".
func (c *Config) SecretName(short string) string { return c.SecretPrefix + "/" + short }

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

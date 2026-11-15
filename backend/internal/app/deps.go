package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/redis/go-redis/v9"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/cache"
	"athlix/backend/internal/platform/config"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/eligibility"
	"athlix/backend/internal/platform/entitlements"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/media"
	"athlix/backend/internal/platform/ratelimit"
	"athlix/backend/internal/platform/requirements"
	"athlix/backend/internal/platform/secrets"
	"athlix/backend/internal/platform/settings"
	"athlix/backend/internal/platform/sms"
	"athlix/backend/internal/platform/storage"
	"athlix/backend/internal/platform/userdir"
)

// Secret short names (full name = Cfg.SecretName(short), e.g. "athlix/prod/db").
// Shapes are documented in docs/security.md#secrets.
const (
	SecretDB            = "db"
	SecretJWT           = "jwt"
	SecretPIIKeys       = "pii-keys"
	SecretRazorpay      = "razorpay"
	SecretDigiLocker    = "digilocker"
	SecretKYCAggregator = "kyc-aggregator"
	SecretMSG91         = "msg91"
	SecretFCM           = "fcm"
	SecretGooglePlay    = "google-play"
	SecretAppStore      = "app-store"
)

// Deps is everything a module may use. Built once per process by Bootstrap.
type Deps struct {
	Cfg          *config.Config
	DB           *sql.DB
	Redis        *redis.Client
	Settings     *settings.Store       // app_config (business switches/limits, no deploy needed)
	Crypto       *crypto.Box           // PII encryption + lookup hashes
	Secrets      secrets.Getter        // Secrets Manager (cached); use Cfg.SecretName(short)
	Storage      storage.Storage       // private media bucket (only via Media unless you know why)
	Media        *media.Service        // uploads, claims, signed view URLs
	SMS          sms.Sender            // OTP SMS (provider chosen by auth.sms_provider)
	Events       events.Publisher      // publish domain events after commit
	Dispatcher   *events.Dispatcher    // handlers registered from modules (local + worker)
	Users        *userdir.Directory    // read-only user lookups, summaries, blocks/follows
	Entitlements *entitlements.Checker // premium benefits
	Requirements *requirements.Checker // verification_requirements gates
	Eligibility  *eligibility.Checker  // location-based event eligibility (the ONE rule implementation)
	Tokens       *authn.Issuer         // access tokens
	Limiter      *ratelimit.Limiter    // Redis fixed-window rate limits
	Log          *slog.Logger
	AWS          *aws.Config // nil when ENV=local
	Now          func() time.Time
}

// LocalStorage is set when ENV=local so the router can mount the dev storage routes.
var LocalStorage *storage.Local

// Bootstrap builds Deps from Config. cleanup closes DB/Redis.
func Bootstrap(ctx context.Context, cfg *config.Config) (*Deps, func(), error) {
	log := logger.New(os.Getenv("LOG_LEVEL"))
	slog.SetDefault(log)
	d := &Deps{Cfg: cfg, Log: log, Now: time.Now}

	var getter secrets.Getter
	if cfg.IsLocal() {
		lf, err := secrets.NewLocalFile(envOr("SECRETS_FILE", ".secrets.local.json"))
		if err != nil {
			return nil, nil, fmt.Errorf("local secrets: %w (run `make dev-secrets`)", err)
		}
		getter = lf
	} else {
		ac, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
		if err != nil {
			return nil, nil, errors.New("aws config load failed")
		}
		d.AWS = &ac
		getter = secrets.NewAWS(ac)
	}
	d.Secrets = getter

	dsn := cfg.DBDSN
	if !cfg.IsLocal() || dsn == "" {
		var s db.Secret
		if err := getter.GetJSON(ctx, cfg.SecretName(SecretDB), &s); err != nil {
			return nil, nil, err
		}
		dsn = s.WithHost(cfg.DBHostOverride).DSN()
	}
	conn, err := db.Open(dsn, 10)
	if err != nil {
		return nil, nil, errors.New("database open failed")
	}
	d.DB = conn

	rdb, closeRedis, err := cache.New(cache.Options{Addr: cfg.RedisAddr, TLS: cfg.RedisTLS})
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	d.Redis = rdb
	cleanup := func() { closeRedis(); conn.Close() }

	var pk crypto.Keys
	if err := getter.GetJSON(ctx, cfg.SecretName(SecretPIIKeys), &pk); err != nil {
		cleanup()
		return nil, nil, err
	}
	if d.Crypto, err = crypto.NewBox(pk); err != nil {
		cleanup()
		return nil, nil, err
	}
	var jk authn.SigningKeys
	if err := getter.GetJSON(ctx, cfg.SecretName(SecretJWT), &jk); err != nil {
		cleanup()
		return nil, nil, err
	}
	if d.Tokens, err = authn.NewIssuer(jk, "athlix-"+string(cfg.Env)); err != nil {
		cleanup()
		return nil, nil, err
	}

	d.Settings = settings.New(conn, 15*time.Second)
	d.Limiter = ratelimit.New(rdb)
	d.Entitlements = entitlements.New(conn)
	d.Requirements = requirements.New(conn, 15*time.Second)
	d.Eligibility = eligibility.New(conn, d.Settings)

	if cfg.IsLocal() {
		key := make([]byte, 32)
		_, _ = rand.Read(key)
		LocalStorage = storage.NewLocal(cfg.LocalStorageDir, cfg.PublicAPIBaseURL, key)
		d.Storage = LocalStorage
	} else {
		d.Storage = storage.NewS3(*d.AWS, cfg.MediaBucket)
	}
	d.Media = media.New(conn, d.Storage, d.Settings)
	d.Users = userdir.New(conn, d.Media)

	d.Dispatcher = events.NewDispatcher()
	if cfg.EventsQueueURL == "" {
		d.Events = &events.Local{D: d.Dispatcher}
	} else {
		d.Events = events.NewSQS(*d.AWS, cfg.EventsQueueURL)
	}

	providers := map[string]sms.Sender{}
	if !cfg.IsProd() {
		providers["stub"] = sms.Stub{}
	}
	var ms sms.MSG91Secret
	if err := getter.GetJSON(ctx, cfg.SecretName(SecretMSG91), &ms); err == nil && ms.AuthKey != "" {
		providers["msg91"] = sms.NewMSG91(ms)
	}
	d.SMS = &sms.Selector{Providers: providers, Current: func(ctx context.Context) string {
		return d.Settings.String(ctx, "auth.sms_provider", "stub")
	}}
	return d, cleanup, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

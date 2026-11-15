# backend/internal/platform

Technical building blocks shared by every module. No business rules live here: anything a
business person might want to change belongs in `app_config` or a config table instead.
Each package has its own README (generated from its doc comments).

| Package | One line |
|---|---|
| [audit](audit/README.md) | write `admin_audit_log` / `config_audit` rows |
| [authn](authn/README.md) | JWTs, auth middleware, caller identity from the context |
| [cache](cache/README.md) | Redis client (miniredis locally) |
| [config](config/README.md) | environment variables → `Config` |
| [crypto](crypto/README.md) | AES-256-GCM PII encryption, HMAC lookup hashes, masking |
| [db](db/README.md) | MySQL open, transactions, error helpers, placeholders |
| [entitlements](entitlements/README.md) | premium benefits of a user |
| [events](events/README.md) | domain event bus (in-process / SQS) and payload structs |
| [httpx](httpx/README.md) | response envelope, typed errors, binding, cursor pagination |
| [logger](logger/README.md) | JSON slog logger with request id and redaction |
| [media](media/README.md) | upload tickets, claims, signed view URLs |
| [ratelimit](ratelimit/README.md) | Redis fixed-window limiter |
| [requirements](requirements/README.md) | `verification_requirements` gates |
| [secrets](secrets/README.md) | AWS Secrets Manager (or local file) |
| [settings](settings/README.md) | typed reads of `app_config` |
| [sms](sms/README.md) | OTP SMS (stub, MSG91) |
| [storage](storage/README.md) | S3 / local disk object storage |
| [userdir](userdir/README.md) | read-only user summaries, blocks, follows |

Added during the build: `razorpay` (listing payments) and `push` (FCM HTTP v1).

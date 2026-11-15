# backend/

Go 1.24 modular monolith: one module (`athlix/backend`), four Lambda binaries.
Read [docs/architecture.md](../docs/architecture.md) first.

```
cmd/          binaries: api, ws, worker, migrate, admincli, devsecrets
internal/app  Deps, Bootstrap, router, module interfaces
internal/platform  shared building blocks (db, authn, httpx, crypto, settings, events, media ...)
internal/modules   feature modules (auth, user, event, chat, ...)
internal/testutil  test helpers
migrations/   SQL schema + seed (embedded into cmd/migrate)
scripts/smoke end-to-end smoke test against a local API (Python, standard library)
tools/        pinned dependency list
```

## Run locally

Needs Go 1.24+, and MySQL 8 + Redis 7 (`docker compose up -d` in the repo root).

```bash
cd backend
make dev-secrets         # random local keys -> .secrets.local.json (gitignored)
make migrate             # schema + seed config
make run                 # http://localhost:8080/v1/health
make create-admin EMAIL=you@example.com NAME="You"   # prints a one-time password + TOTP URI
```

Locally: `ENV=local`, SMS is a stub (OTP is logged only as "sent"; `DEV_FIXED_OTP=123456`
works), DigiLocker / payments / store purchases use **mock** providers
(`verification.provider`, `listing.payment_provider` = `mock`; mock endpoints exist only when
`ENV≠prod`), media is stored under `.localstorage/`, events run in-process, chat WebSocket is
`ws://localhost:8080/v1/chat/ws`. Without `REDIS_ADDR`, an in-memory miniredis is used. Every DB session runs in UTC
(`db.Open` forces `loc=UTC` and `time_zone='+00:00'`), whatever the local MySQL default is.

### Smoke test

```bash
B=http://localhost:8080/v1
R=$(curl -s -XPOST $B/auth/otp/request -d '{"mobile":"9876543210"}' | jq -r .data.requestId)
T=$(curl -s -XPOST $B/auth/otp/verify -d "{\"requestId\":\"$R\",\"otp\":\"123456\"}" | jq -r .data.tokens.accessToken)
curl -s $B/me -H "Authorization: Bearer $T" | jq .
curl -s "$B/listing/quote?sportId=1&eventType=tournament&entryFeePaise=50000&maxSlots=16" -H "Authorization: Bearer $T"
```

(The quote needs a finished onboarding: role → `POST /verification/mock/complete
{"outcome":"adult"}` → profile.)

Full end-to-end run (onboarding, organiser approval, fee from config, config change, listing
payment, location eligibility, ownership): [scripts/smoke](scripts/smoke/README.md).

### Environment

| Var | Default | Meaning |
|---|---|---|
| `ENV` | `local` | `local` \| `dev` \| `prod` |
| `AWS_REGION` | `ap-south-1` | |
| `SECRET_PREFIX` | `athlix/local` | Secrets Manager prefix (`athlix/prod`) |
| `DB_DSN` | (none) | local only; in AWS the `db` secret is used |
| `REDIS_ADDR` | `miniredis` | `host:port`, or `miniredis` for in-memory |
| `MEDIA_BUCKET` | | S3 bucket (AWS) |
| `LOCAL_STORAGE_DIR` | `.localstorage` | local media folder |
| `EVENTS_QUEUE_URL` | | SQS queue; empty = in-process |
| `WS_MANAGEMENT_URL` | | API Gateway WS management endpoint (`https://...execute-api.../<stage>`) |
| `WS_PUBLIC_URL` | `ws://localhost:8080/v1/chat/ws` | returned by `/chat/ws-ticket` |
| `PUBLIC_API_BASE_URL` | `http://localhost:8080` | used for callbacks / local media URLs |
| `APP_DEEP_LINK_BASE` | `athlix://` | DigiLocker result deep link |
| `ADMIN_ORIGINS` | `http://localhost:5050` | CORS for the admin panel (comma separated) |
| `HTTP_ADDR` | `:8080` | local listener |
| `DEV_FIXED_OTP` | | **local only**: accept this OTP |
| `LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |

No secret value is ever an environment variable; only names/prefixes.

## Checks (CI runs these)

```bash
gofmt -l .        # must print nothing
go vet ./...
go test ./...     # sqlmock + miniredis, no external services
make build-lambda # arm64 zips in dist/
```

## Writing a feature

1. Contract first: add/modify the endpoint in `docs/api.md`.
2. Tables in a new migration; ownership in `docs/architecture.md`.
3. Code in the owning module (handler → service → repository); limits from `d.Settings`.
4. Tests: service rules with sqlmock / `testutil`, handler status codes and envelopes,
   ownership (other user's id → 404), validation (422), config-driven limits.
5. Update the module README.

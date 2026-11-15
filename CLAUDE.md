# CLAUDE.md: rules for AI coding agents (and humans) in the Athlix repo

Read this file first, then the README.md of the folder you are working in. Every folder has
a README.md that says what it owns and how to change it safely. **Keep those READMEs true:**
if you change what a folder does, update its README in the same change.

## What Athlix is

A sports community app for India: players, coaches and tournament organisers. Profiles with
verified identity (DigiLocker), tournament listings, registrations, 1:1 and group chat, a
feed, coach consultations and premium plans. Product details: [docs/business/product-overview.md](docs/business/product-overview.md).

**Athlix never holds player money.** Entry fees go directly from player to organiser (cash,
UPI, bank). Athlix earns a **listing fee** from the organiser when they publish a tournament,
plus premium subscriptions and ads. Never add code that collects, holds or splits an entry fee.

## Repo map

| Path | What | Read |
|---|---|---|
| `backend/` | Go API, WebSocket, worker, migrations | [backend/README.md](backend/README.md) |
| `app/` | Flutter mobile app (Android, iOS) | [app/README.md](app/README.md) |
| `admin/` | Flutter Web admin panel | [admin/README.md](admin/README.md) |
| `infra/` | Terraform for AWS (ap-south-1) | [infra/README.md](infra/README.md) |
| `docs/` | Contracts and guides | [docs/README.md](docs/README.md) |

Contracts every part builds against (change these first, then the code):

- [docs/api.md](docs/api.md): every HTTP endpoint and WebSocket message
- [docs/data-model.md](docs/data-model.md) + `backend/migrations/*.sql`: every table
- [docs/config-system.md](docs/config-system.md): every runtime setting and config table
- [docs/architecture.md](docs/architecture.md): modules, ownership, domain events, jobs
- [docs/security.md](docs/security.md): security rules, secrets, admin permissions

## Hard rules (do not break; if a task needs to, stop and ask)

1. **No business number in code.** Fees, percentages, limits, durations, switches, plan
   prices and benefits, refund percentages and verification steps come from the DB
   (`app_config` via `d.Settings`, or the typed config tables). Code holds only a fallback
   default that equals the seed value. New setting = new row in a new migration + a line in
   docs/config-system.md.
2. **Every route is on an authenticated group.** Use `app.Routes`: `Verified` for almost
   everything, `User` only for onboarding, `Admin` + `authn.RequirePermission(...)` for the
   admin panel. `Public` only for: health, bootstrap config, policies, sports list, OTP,
   token refresh, the DigiLocker callback (bound by one-time state). `Webhooks` handlers verify
   the provider signature before anything else.
3. **The caller comes from the token only:** `authn.UserID(c)` / `authn.AdminID(c)`. Never
   trust a user id from the body, query or path as "who is calling".
4. **Ownership in the query.** Load user-owned rows with the owner in the WHERE clause
   (`WHERE id = ? AND organiser_id = ?`). On mismatch return **404** (`httpx.NotFound`),
   the same as "does not exist".
5. **Parameterised SQL only.** `?` placeholders; `db.Placeholders(n)` for IN lists; dynamic
   ORDER BY through a hard-coded map. Never `fmt.Sprintf` / `+` a value into SQL.
6. **No PII or secrets in logs, errors or responses.** Log ids only. The logger redacts
   sensitive keys, but don't rely on it. Never log request/response bodies.
7. **PII at rest:** mobile, guardian name/mobile, organiser contact phone, DigiLocker
   name/DOB are encrypted with `d.Crypto` (AES-256-GCM, `_enc` column + `key_version`),
   looked up by `d.Crypto.Hash(purpose, value)` (`_hash`), displayed from `_last4`.
   **Never store the full Aadhaar number, Aadhaar XML/PDF, or Aadhaar photo.**
8. **Secrets only in AWS Secrets Manager** (`d.Secrets.GetJSON(ctx, d.Cfg.SecretName("razorpay"), &s)`).
   Env vars / app_config hold secret *names*, never values. Nothing secret in the repo, the
   apps, Terraform state outputs, or CI variables.
9. **Files:** clients never send S3 keys. Upload through `d.Media.CreateUpload` (backend
   builds the key), attach with `d.Media.Claim` (checks owner + purpose), show with
   `d.Media.ViewURL(s)` (short-lived signed URL) **after** the viewer passed the access check.
10. **Money is integer paise** (`int64`, JSON number). Never float.
11. **Minors:** respect `is_minor` everywhere a stranger could reach a child: DMs
    (`minor.dm_policy`), search (`minor.searchable`), ads (`ads.show_to_minors`), default
    profile visibility. When unsure, choose the safer behaviour.
12. **Only the owning module writes a table** (see docs/architecture.md#modules-and-owners). Other
    modules may read it. Cross-module reactions go through domain events (`d.Events`).

## Backend conventions (Go)

- Module layout: `internal/modules/<name>/{module.go, handler.go, service.go, repository.go, model.go, *_test.go, README.md}`.
  Handler = HTTP binding + response shape only. Service = rules. Repository = SQL only.
- Register the module once in `internal/modules/registry.go`. Nothing else is shared.
- Responses: `httpx.OK / Created / Page / NoContent`, errors: `httpx.Fail(c, httpx.NotFound("Event"))`.
  JSON is camelCase; ids are numbers; times RFC 3339 UTC; money `...Paise`.
- Pagination: cursor based (`httpx.ParsePage`, `httpx.EncodeCursor`), `meta.nextCursor`.
- Publish domain events **after** the transaction commits, using the payload structs in
  `internal/platform/events/payloads.go`. Handlers must be idempotent.
- Tests: `internal/testutil` gives a full `*app.Deps` with sqlmock, miniredis, static
  settings and in-memory storage. Every service rule gets a test; every handler that takes
  an id gets an ownership (404) test.
- Do not run `go get` or edit go.mod casually: dependencies are pinned in `backend/tools/deps.go`.
- Before you finish: `cd backend && gofmt -l . && go vet ./... && go test ./...` all clean.

## App and admin conventions (Flutter)

- Feature-first folders (`lib/features/<feature>/{data,application,presentation}`), Riverpod for
  state, go_router for navigation, dio for HTTP with the auth interceptor in `lib/core/api`.
- Models mirror docs/api.md exactly (field names, nullability). If the API doc and the code
  disagree, the doc wins: fix the code or change the doc and backend together.
- Never store tokens anywhere except `flutter_secure_storage`. Never log tokens or PII.
- No hard-coded business numbers in the UI: read limits from `GET /v1/bootstrap` config.

## Git

- Small commits with a plain sentence message ("Add listing fee quote endpoint").
- Never commit `.secrets.local.json`, `.env*`, keystores, `google-services.json`
  with real keys, Terraform state, or exports of production data.

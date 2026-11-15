# Architecture

## Shape

```
 Flutter app ──HTTPS──► API Gateway HTTP API ──► Lambda  api     (cmd/api, gin, every REST route)
      │                                              │
      └──WSS──► API Gateway WebSocket ──► Lambda  ws  (cmd/ws: $connect/$disconnect/sendMessage/typing/markRead/ping)
                                                     │
 Flutter Web admin ──HTTPS──► same HTTP API (/v1/admin/*)
                                                     │
            ┌──────────── RDS MySQL 8 ◄──────────────┤
            │             ElastiCache Redis ◄────────┤  (rate limits, OTP, WS connections, tickets)
            │             S3 private media ◄─────────┤  (presigned PUT/GET only)
            │             Secrets Manager ◄──────────┤  (all keys)
            │                                        │ publish after commit
            │             SQS athlix-<env>-events ◄──┘
            │                    │
            └──────────── Lambda worker (cmd/worker) ◄── EventBridge Scheduler (jobs)
                                 │
                                 └──► FCM push, API Gateway WS management API, SMS
```

- One Go module (`backend/`), a **modular monolith**: feature modules under
  `internal/modules/<name>`, built into four binaries: `api`, `ws`, `worker`, `migrate`
  (+ `admincli`, `devsecrets` for operators/dev).
- Locally everything runs in **one process** (`cmd/api`): in-process event bus, miniredis
  (unless `REDIS_ADDR` is set), local disk storage, local WebSocket at `/v1/chat/ws`.
- Region `ap-south-1` (Mumbai) for data residency.

## Module contract

```go
type Module interface { Name() string; Routes(r app.Routes) }
// optional
EventHandlers() map[string]events.Handler // EventSubscriber
Jobs() map[string]app.JobFunc            // JobProvider
```

- `New(d *app.Deps) *Module` builds it; `internal/modules/registry.go` lists every module
  (the only shared file; already complete).
- Layout per module: `module.go` (New/Name/Routes/handlers wiring), `handler.go` (HTTP:
  bind → call service → respond), `service.go` (rules), `repository.go` (SQL),
  `model.go` (DB structs + JSON DTOs), `*_test.go`, `README.md`.
- A module may **read** any table (for display / checks) but **writes only the tables it owns**
  (below). To change another module's data, publish an event and let the owner react.

## Modules and owners

The build was split between six agents. "Agent" is kept here so future changes know which
modules were designed together.

| Module | Agent | Owns tables | Purpose |
|---|---|---|---|
| auth | A | `refresh_tokens`, `users` (insert on signup, `last_login_at`) | OTP login, JWT, refresh rotation |
| user | A | `users` (profile/onboarding/status columns), `user_roles`, `user_settings`, `policy_acceptances` | /me, roles, settings, account delete, admin users |
| verification | A | `identity_verifications`, `users.verification_status/is_minor/onboarding_step` | DigiLocker (direct / aggregator / mock) |
| guardian | A | `guardian_consents`, `users.guardian_consent_status/onboarding_step` | parent consent OTP for minors |
| sport | A | (read-only) `sports`, `sport_positions`, `stat_templates` | public sports catalogue |
| profile | A | `profiles`, `profile_sports`, `user_stats`, `achievements`, `profile_views` | profile, stats, achievements, viewers |
| follow | A | `follows`, `blocks` | follow / block |
| organiser | A | `organiser_profiles`, `organiser_verification_submissions` | organiser verification + admin review |
| appconfig | A | `app_config`, `policy_documents`, `verification_requirements`, `organiser_verification_steps`, writes to `sports`, `sport_positions`, `stat_templates` | public bootstrap + admin config editors |
| admin | A | `admin_users`, `admin_roles` | admin login (password + TOTP), admins, roles, dashboard, audit log view |
| media | A | `media_uploads` (via `platform/media`) | upload tickets |
| event | B | `events` | event CRUD, discovery, contact reveal, featuring, admin review |
| listingpayment | B | `listing_fee_rules`, `listing_orders`, `payment_webhook_events` (provider `razorpay`), `events.status` on payment | listing fee quote/order/confirm/webhook/refund |
| registration | B | `event_registrations`, `events.registrations_count` | register, waitlist, withdraw, organiser marks payment |
| subscription | B | `premium_plans`, `premium_plan_benefits`, `subscriptions`, `payment_webhook_events` (providers `google_play`, `app_store`) | plans, store verification, store webhooks |
| consultation | B | `coach_consultations`, `consultation_requests` | coach consultation listing and requests |
| moderation | B | `reports`, `moderation_actions`, + **status columns** of reported tables | reports, auto-hide, admin resolution |
| search | B | (read-only) | people / global search |
| ads | B | `ad_campaigns`, `ad_daily_stats` | contextual sponsored placements |
| chat | C | `conversations` (`direct`, `event`), `conversation_members`, `messages` | DMs, event groups, messages, WebSocket |
| community | C | `conversations` (`community`), `conversation_members` of communities | user-created groups |
| notification | C | `notifications`, `device_tokens` | in-app + FCM push, reminders, broadcast |
| feed | C | `posts`, `post_likes`, `post_comments` | posts, likes, comments |
| locationidentity | G | `states`, `districts`, `user_location_identities` | native / residence identities, proof review, districts admin, expiry + purge jobs |

Append-only shared tables (any module, through the helper only):
`admin_audit_log` via `audit.Admin(c, q, action, targetType, targetID, details)`;
`config_audit` via `audit.Config(ctx, q, table, recordID, old, new, adminID)`.

**The two exceptions** to "only the owner writes":
1. `moderation` may update the `status` column of `users`, `events`, `posts`,
   `post_comments`, `messages`, `conversations` (hide / remove / suspend). It never touches
   other columns. Owners must treat `hidden`/`removed`/`suspended` as invisible.
2. `registration` updates `events.registrations_count` (atomically, see api.md).

Modules owned by **different agents** interact only through events + read-only table reads,
never by importing each other's Go packages. Shared reads go through `platform/userdir`
(user summary, blocks, follows), `platform/entitlements`, `platform/requirements`,
`platform/settings`, `platform/media`, `platform/eligibility`.

## Platform packages (`backend/internal/platform`)

| Package | What it gives you |
|---|---|
| `config` | env vars → `Config` (`ENV`, `SECRET_PREFIX`, `DB_DSN`, `REDIS_ADDR`, `MEDIA_BUCKET`, `EVENTS_QUEUE_URL`, `WS_MANAGEMENT_URL`, `WS_PUBLIC_URL`, `PUBLIC_API_BASE_URL`, `APP_DEEP_LINK_BASE`, `ADMIN_ORIGINS`, `HTTP_ADDR`, `DEV_FIXED_OTP`) |
| `secrets` | Secrets Manager (cached in memory) or `.secrets.local.json` locally |
| `db` | `Open`, `Querier`, `WithTx`, `IsNoRows`, `IsDuplicate`, `Placeholders(n)`, `Int64Args`, `NullString`, `NullInt64` |
| `cache` | Redis client (miniredis locally) |
| `crypto` | `Box`: AES-256-GCM `Encrypt/EncryptString/Decrypt/DecryptString`, `Hash(purpose, v)` HMAC, `KeyVersion`; `SHA256`, `RandomToken`, `Last4`, `MaskMobile` |
| `httpx` | envelopes `OK/Created/Page/NoContent/Fail`, typed errors, `BindJSON`, `PathID`, `QueryInt64`, `QueryBool`, `ParsePage`, `EncodeCursor` |
| `authn` | JWT issuer, `UserAuth`, `RequireOnboarded`, `AdminAuth`, `RequirePermission`, `UserID(c)`, `User(c)`, `AdminID(c)`, test setters |
| `settings` | typed `app_config` reads (`Int/Bool/String/StringList/JSON/Seconds/Public/All`), 15 s cache |
| `requirements` | `verification_requirements` gates (`Required(ctx, role, requirement, gate)`) |
| `entitlements` | premium benefits of a user (`Benefits`, typed helpers) |
| `userdir` | read-only user lookups: `Get`, `Summary`, `Summaries`, `BlockedEitherWay`, `Follows`, `MutualFollow`, `Invalidate` |
| `media` | `CreateUpload`, `Confirm`, `Claim(q, userID, uploadID, purpose)`, `ViewURL`, `ViewURLs`, `OwnerOf` |
| `storage` | S3 / local disk behind one interface (used by `media`) |
| `events` | domain event bus (`Publisher`, `Dispatcher`, `Local`, `SQS`) + payload structs |
| `ratelimit` | `Limiter.Allow(ctx, key, limit, window)` |
| `sms` | OTP SMS sender (`stub` locally, MSG91) |
| `logger` | slog JSON logger with request id and PII/secret redaction |
| `audit` | admin + config audit rows |

Added by the agents: `platform/razorpay` (B), `platform/push` (C, FCM HTTP v1).

## Domain events

Published **after** the DB transaction commits (`d.Events.Publish`). Locally dispatched in
process; in AWS sent to SQS `athlix-<env>-events` and dispatched by `cmd/worker`. Delivery is
**at least once**: every handler must be idempotent. Payloads (`platform/events/payloads.go`)
hold ids and flags only.

| Type | Publisher | Payload | Consumers |
|---|---|---|---|
| `user.registered` | auth | UserPayload | (analytics later) |
| `user.deleted` | user | UserPayload | locationidentity (withdraw identities, delete proofs), follow (remove follows/blocks), profile, organiser, verification, guardian, chat (hide the user's messages), notification (delete the user's device tokens) |
| `verification.completed` | verification | VerificationPayload (+ `nativeDistrictId`) | notification, locationidentity (create the DigiLocker native identity) |
| `guardian.consent_granted` | guardian | UserPayload | notification |
| `guardian.consent_revoked` | guardian (guardian withdrawal or admin revoke) | UserPayload | notification (user: consent withdrawn, back to the guardian step) |
| `follow.created` | follow | FollowPayload | notification |
| `organiser.approved` / `.rejected` | organiser | OrganiserPayload | notification |
| `event.submitted` | listingpayment / event | EventPayload | notification (organiser: "in review") |
| `event.published` | event / listingpayment | EventPayload | chat (create event group if `event.auto_create_group`), notification |
| `event.rejected` | event | EventPayload | notification, listingpayment (refund `on_admin_rejection_percent`) |
| `event.cancelled` | event | EventPayload | chat (archive group), notification (all registered players), listingpayment (refund) |
| `event.updated` | event | EventPayload | notification (registered players) |
| `event.completed` | event (job) | EventPayload | chat (archive group) |
| `registration.created` | registration | RegistrationPayload | chat (add to event group if `registered`), notification (organiser) |
| `registration.withdrawn` | registration | RegistrationPayload | chat (remove from group), notification (organiser) |
| `registration.promoted` | registration | RegistrationPayload | chat (add to group), notification (player) |
| `registration.payment_marked` | registration | RegistrationPayload | notification (player) |
| `listing.paid` | listingpayment | ListingPayload | notification |
| `listing.refunded` | listingpayment | ListingPayload | notification |
| `message.created` | chat | MessagePayload | notification (push only to offline, unmuted members; no in-app row per message) |
| `post.liked` / `post.commented` | feed | PostPayload | notification |
| `consultation.requested` / `.responded` | consultation | ConsultationPayload | notification |
| `report.created` | moderation | ReportPayload | (admin dashboard count) |
| `subscription.activated` / `.expired` | subscription | SubscriptionPayload | notification |
| `community.join_requested` / `.join_approved` | community | CommunityJoinPayload | notification |
| `community.ownership_transferred` | community | OwnershipPayload (new owner, previous owner) | (none yet; the transfer row is the audit) |
| `location_identity.changed` | locationidentity | LocationIdentityPayload | registration (re-check + flag), notification (user: verified / rejected / expired / revoked) |
| `location_identity.expiring` | locationidentity (job) | LocationIdentityPayload | notification (user: renew residence) |
| `registration.eligibility_flagged` | registration | RegistrationPayload (+ `eligibilityFlag`) | notification (organiser and player) |
| `notification.broadcast_push` | notification (internal, to itself) | page of recipient ids | notification (sends an admin broadcast in pages; idempotent per recipient) |

Who publishes what for listing payment: when payment completes, `listingpayment` updates
`events.status` (it owns that transition) and publishes `listing.paid` plus `event.submitted`
or `event.published`. Admin approve/reject (in `event`) publishes `event.published` /
`event.rejected`. Cancel (in `event`) publishes `event.cancelled`; `listingpayment` handles
the refund from that event, reading the event row: status `cancelled` (organiser) → refund
per `listing.refund.before_first_registration_percent` / `after_registrations_percent`;
status `removed` (admin, rule violation) → no refund. Refunds are idempotent per order.

### Notifications

The `notification` module maps every event above to a row in `notifications` and, if the
user's `notificationPrefs` category is on, an FCM push. Categories: `messages`
(message.created), `events` (event.*, registration.*, listing.*, organiser.*), `social`
(follow, post, community, consultation), `marketing` (admin broadcast). Text never contains
mobile numbers or other PII; titles use display names only. `data` holds deep-link ids.

## Scheduled jobs

EventBridge Scheduler invokes the worker Lambda with `{"job": "<name>"}`. Locally:
`go run ./cmd/worker -job <name>`. Jobs must be idempotent and batch (≤ 500 rows per run).

| Job | Module | Schedule | Does |
|---|---|---|---|
| `auth.cleanup_tokens` | auth | daily 03:00 IST | delete refresh tokens expired or revoked more than 7 days ago |
| `verification.minor_recheck` | verification | daily 04:00 IST | users whose 18th birthday passed: `is_minor=0` |
| `media.cleanup_pending` | media | hourly | delete unconfirmed uploads older than 24 h (object + row) |
| `event.complete_past` | event | hourly | `published` events whose `ends_at` passed → `completed`, emit `event.completed` |
| `listing.expire_orders` | listingpayment | every 15 min | `created` orders past `expires_at` → `expired`, event → `draft` |
| `subscription.expire` | subscription | hourly | past `expires_at` → `expired` (grace handling), emit `subscription.expired` |
| `location.expire_residence` | locationidentity | daily 00:30 IST | verified residence past `expires_at` → `expired`, emit `location_identity.changed` |
| `location.expiry_reminders` | locationidentity | daily 10:00 IST | verified residence expiring within `location.expiry_reminder_days`, not yet reminded → set `expiry_reminded_at`, emit `location_identity.expiring` |
| `location.purge_proofs` | locationidentity | daily 02:30 IST | reviewed rows older than `location.proof_retention_days` (and withdrawn rows) with a proof → `media.Purge`, set `proof_purged_at`, `proof_upload_id = NULL` |
| `notification.event_reminders` | notification | hourly | remind registered players `notification.event_reminder_hours` before start (deduped in Redis) |
| `admin.purge_login_events` | admin | daily 03:15 IST | delete `admin_login_events` older than `admin.login_events_retention_days` |
| `verification.purge_subject_hash` | verification | daily 03:45 IST | users deleted more than `verification.subject_hash_retention_days` ago: clear `subject_hash` and `provider_ref` |
| `ads.end_campaigns` | ads | hourly at :25 | active / paused campaigns past `ends_at` → `ended` (serving already stops at `ends_at`) |

## Real-time chat

- App calls `POST /v1/chat/ws-ticket` → `{ticket, url}`; ticket is random, stored in Redis
  `ws:ticket:<t>` → userId for `chat.ws_ticket_ttl_seconds`, single use.
- AWS: `$connect` (`cmd/ws`) redeems the ticket, stores `ws:conn:<connectionId>` → userId and
  `ws:user:<userId>` set of connection ids (TTL 2 h, refreshed on ping). `$disconnect` removes
  them. Messages are saved in MySQL, then posted to every online member's connections through
  the API Gateway Management API (`WS_MANAGEMENT_URL`); `410 Gone` connections are removed.
- Local: `cmd/api` mounts `GET /v1/chat/ws` (coder/websocket) on `Routes.Engine` with the same
  ticket check and an in-memory hub. Same JSON frames as AWS.
- Sending logic is one service function used by REST, the WS route and `cmd/ws`.

## Deployment

| Lambda | Binary | Trigger | Memory / timeout |
|---|---|---|---|
| `athlix-<env>-api` | `cmd/api` | HTTP API `$default` | 512 MB / 29 s |
| `athlix-<env>-ws` | `cmd/ws` | WebSocket API routes | 256 MB / 29 s |
| `athlix-<env>-worker` | `cmd/worker` | SQS `athlix-<env>-events` (batch 10, DLQ after 5) + Scheduler | 512 MB / 120 s |
| `athlix-<env>-migrate` | `cmd/migrate` | invoked by `terraform apply` (aws_lambda_invocation, re-runs when the migrate zip changes) | 256 MB / 300 s |

All Lambdas: `provided.al2023`, arm64, in private subnets (RDS + Redis), egress through one
NAT gateway (DigiLocker, Razorpay, FCM, SMS). IAM per function, scoped to its secrets, queue,
bucket and WS API. Build: `make build-lambda` → `backend/dist/<name>.zip`. Terraform in
`infra/` (see [aws-setup.md](aws-setup.md)).

Scale target: 10k–1 lakh users. A `db.t4g.small`/`medium` RDS and a `cache.t4g.micro`
Redis are enough; see [business/costs.md](business/costs.md).

## Location eligibility

Tournaments can be limited to players **from** a place (native, from official ID) or
**living in** it (residence, from an address proof), at district or state level.

```
organiser ── EventInput.category + eligibility ──▶ event (events.eligibility_*)
player ── DigiLocker ──▶ verification.completed{nativeDistrictId} ──▶ locationidentity: verified native
player ── upload proof ──▶ locationidentity: pending ──admin review──▶ verified | rejected
feed / detail / register ──▶ platform/eligibility (Filter SQL / Evaluate) ◀── user_location_identities
identity stops being valid ──▶ location_identity.changed ──▶ registration re-check ──▶ flag (never cancel)
```

- **One rule implementation**: `platform/eligibility` (`Evaluate` for one event, `Filter` for
  feed SQL, `Describe` / `Tag` for text, `Places` cache + DigiLocker district matching). The
  event, registration and search modules call it; nobody re-implements it.
- **Tables**: `locationidentity` owns `states`, `districts`, `user_location_identities`. The
  generated `verified_slot` / `pending_slot` columns with UNIQUE keys enforce "one verified
  and one pending per type". History rows stay (`superseded`, `expired`, `rejected`, `withdrawn`).
- **Privacy**: no address is stored, only the district. Proof files live in the private
  media bucket under `location_proof/<userId>/<uuid>.<ext>` (backend-built key), are visible only to
  reviewers with `location.review` through short signed URLs (audited), and are deleted
  `location.proof_retention_days` after review. Aadhaar: last 4 digits at most.
- **Flag, never cancel**: when an identity leaves `verified` (or a pending one used under
  `requireVerified: false` is rejected), `registration` re-evaluates affected registrations of
  events not yet started and sets `eligibility_flag` if nothing else qualifies.
- **Payment**: Athlix does not take entry fees, so "re-check before payment" means the
  contact / payment-instructions reveal re-checks eligibility (403 `NOT_ELIGIBLE`).
- **Switch**: `location.enabled = false` makes every event behave as open.

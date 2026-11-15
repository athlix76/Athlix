# Security

Hard rules (also in the root `CLAUDE.md`). A change that breaks one is not mergeable.

1. **Every route is on an auth group** (`User`, `Verified`, `Admin` + `RequirePermission`,
   or `Webhooks` with signature check). `Public` only for the routes listed in `app/router.go`.
2. **The caller comes from the token** (`authn.UserID(c)`, `authn.AdminID(c)`), never from
   the body, query or path.
3. **Ownership is checked in the query** (`WHERE id = ? AND organiser_id = ?`). Not found
   *or* not yours → `404 NOT_FOUND`, same body, so ids cannot be probed.
4. **SQL uses bind parameters only.** IN lists use `db.Placeholders(n)`; sort columns go
   through an allowlist map; `LIKE` patterns are bound args.
5. **No PII or secrets in logs, errors or responses.** Log ids. The logger redacts known keys
   (`mobile`, `otp`, `token`, `password`, `authorization`, ...), but do not rely on it.
6. **Sensitive PII is encrypted** (AES-256-GCM, `crypto.Box`) in `_enc` columns with
   `key_version`; lookups use `_hash` (HMAC-SHA256, `Box.Hash(purpose, value)`); display uses
   `_last4` / masks. **The full Aadhaar number is never stored, logged or returned.**
7. **Keys live only in AWS Secrets Manager** (locally `.secrets.local.json`, random dev keys,
   gitignored). Not in code, env vars, `app_config`, Terraform state outputs or the app.
8. **The backend builds every S3 key.** Clients get upload tickets and signed URLs only.
9. **Money is integer paise.** Athlix never receives entry fees.
10. **Minor safety rules are enforced in services**, not in the app.

## Secrets

Full name = `<SECRET_PREFIX>/<short>` (e.g. `athlix/prod/razorpay`). Values are JSON.

| Short name | Shape | Used by | Required |
|---|---|---|---|
| `db` | `{"username","password","host","port","dbname"}` | Bootstrap | always (AWS) |
| `jwt` | `{"userSigningKey","adminSigningKey"}` (base64, 32 random bytes each) | authn | always |
| `pii-keys` | `{"current": 1, "keys": {"1": "<base64 32 bytes>"}, "hmacKey": "<base64 32 bytes>"}` | crypto | always |
| `razorpay` | `{"keyId","keySecret","webhookSecret"}` | platform/razorpay | when `listing.payment_provider` = `razorpay` |
| `digilocker` | `{"clientId","clientSecret","redirectUri","authorizeUrl","tokenUrl","apiBaseUrl"}` | verification | when `verification.provider` = `digilocker_direct` |
| `kyc-aggregator` | `{"provider","baseUrl","apiKey","apiSecret","webhookSecret"}` | verification | when `verification.provider` = `aggregator` |
| `msg91` | `{"authKey","otpTemplateId","guardianTemplateId","senderId"}` | sms | when `auth.sms_provider` = `msg91` |
| `fcm` | `{"projectId","clientEmail","privateKey"}` (from the Firebase service account) | platform/push | for push |
| `google-play` | `{"packageName","clientEmail","privateKey","pubsubAudience","pubsubServiceAccountEmail"}` | subscription | when selling on Android |
| `app-store` | `{"bundleId","issuerId","keyId","privateKey","environment": "Sandbox|Production","appAppleId"}` | subscription | when selling on iOS |

- Missing optional secrets disable that provider; the `mock`/`stub` providers are refused
  when `ENV=prod`.
- Rotate PII keys by adding `"2"` to `keys` and setting `current: 2`. Old rows still decrypt;
  a backfill job may re-encrypt. **Never remove a key that rows still use.** `hmacKey` cannot
  rotate without re-hashing every `_hash` column.
- `storeAccountToken` (premium) = UUID formatted from `Box.Hash("store_account", userID)`, so
  no extra secret.

## PII inventory

| Data | Stored as | Shown as |
|---|---|---|
| Mobile | `users.mobile_enc/_hash/_last4`; OTPs in Redis as SHA-256, TTL `auth.otp.ttl_seconds` | `******3210`; full only to the user themself (never in API) |
| Aadhaar | **last 4 only** (`identity_verifications.aadhaar_last4`) | `XXXX XXXX 1234` |
| Name from DigiLocker | `identity_verifications.name_enc` | admin detail only (audited) |
| DOB | `dob_enc`; `profiles.birth_year` plain for age checks | birth year to self only |
| Gender (verified) | `identity_verifications.gender` (M/F/T/U) | never shown; used for event category checks |
| DigiLocker subject id | `subject_hash` only | never |
| Guardian mobile | `guardian_consents.guardian_mobile_enc/_hash/_last4` | masked |
| Event contact phone | `events.contact_phone_enc/_last4` | masked; full via `/events/:id/contact` (rate limited) |
| Admin TOTP secret | `admin_users.totp_secret_enc` | shown once at creation |
| Store purchase token | SHA-256 `purchase_token_hash` | never |
| Refresh tokens / OTP | SHA-256 hash | never |
| Location identity | `user_location_identities`: district id, proof type, status, dates; `aadhaar_last4` for masked Aadhaar only | own app; organisers see only "Native / Residence: <district>" |
| Location proof document | private bucket `location_proof/<userId>/<uuid>`, deleted `location.proof_retention_days` after review (at once if unmasked Aadhaar) | reviewers only (`location.review`), signed URL ≤ 5 min, audited |
| Device location (distance filter) | **not stored**; `lat/lng` per request only, never logged | never |

**Not stored at all:** Aadhaar number, e-Aadhaar XML/PDF, Aadhaar photo, DigiLocker access
/ refresh token, Razorpay/webhook payloads, card/UPI details (Razorpay holds them), FCM
message bodies, the DigiLocker address (only the matched district id is kept), device location.

Account deletion (`DELETE /me`): `status = deleted`, `mobile_enc` and verification
ciphertext overwritten, `mobile_hash` replaced with a random value (the number can sign up
again), display name → "Deleted user", posts/messages hidden, tokens and devices removed.
`identity_verifications.subject_hash` and `provider_ref` are kept for
`verification.subject_hash_retention_days` (default 180, anti-fraud), then cleared by the
`verification.purge_subject_hash` job.
Location identities are withdrawn and any proof files deleted at once (`user.deleted`).

## Minor safety (all config driven)

- Minor = verified age below `guardian.minor_age_years`; below `auth.min_age_years` → refused.
- App access needs guardian OTP consent (`guardian_consent_if_minor`). The guardian can
  withdraw it with a fresh OTP to their own mobile; support can revoke it (`users.write`,
  audited). Either sends the minor back to the guardian step.
- DMs: `minor.dm_policy` (default `mutual_follow`), checked for both directions; consultation
  exception does **not** apply to minors.
- Profile default visibility `minor.profile_default_visibility`; city hidden from
  non-followers; search returns limited fields; no profile-view tracking by minors.
- No ads unless `ads.show_to_minors`; ads are contextual only (no personal targeting).
- Community joins per `minor.can_join_public_communities`; invites follow DM rules.
- `underage_safety` reports are shown first in the admin queue.

## Webhooks

| Route | Verification |
|---|---|
| `/webhooks/razorpay` | `X-Razorpay-Signature` = HMAC-SHA256(raw body, `webhookSecret`), constant-time compare, before parsing |
| `/webhooks/google-play` | `Authorization: Bearer` Pub/Sub OIDC JWT: RS256 against Google certs, `aud` = `pubsubAudience`, `email` = `pubsubServiceAccountEmail`; then re-fetch state from the Play API (never trust the message) |
| `/webhooks/app-store` | `signedPayload` JWS: x5c chain verified to the Apple Root CA G3 (embedded), ES256 signature; then re-fetch from the App Store Server API |

Idempotent on `payment_webhook_events(provider, provider_event_id)`. Respond 200 for
duplicates and ignored types; 400/401 for bad signatures (do not reveal why).

## Admin panel

- Separate JWT key and token type; `AdminAuth` refuses user tokens and vice versa.
- Login = bcrypt (cost 12) password + TOTP; `admin.max_failed_logins` → lock
  `admin.lockout_minutes`. Generic error on any failure.
- Every login attempt is written to `admin_login_events` (admin id or NULL, HMAC of the email,
  IP, user agent, outcome, internal reason; never the password, code or raw email), visible to
  `audit.read` and deleted after `admin.login_events_retention_days`.
- `POST /admin/me/password` needs the current password and a TOTP code, enforces
  `admin.password_min_length`, and revokes every other session of that admin.
- CORS allows only `ADMIN_ORIGINS`. In AWS, put the admin site behind CloudFront and
  (recommended) Cloudflare Access / an IP allowlist.
- Every write → `admin_audit_log`. Viewing a user's verified name or reading reported
  messages is also audited.

### Admin permissions

`"*"` grants everything (role `super_admin`).

| Permission | Allows |
|---|---|
| `dashboard.read` | dashboard counts |
| `admins.manage` | admins and roles |
| `audit.read` | admin audit log |
| `config.read` | read config, policies, fee rules, plans, sports, requirements, steps |
| `config.write` | edit `app_config`, verification requirements, organiser steps |
| `policies.write` | create / activate policy versions |
| `sports.write` | sports, positions, stat templates |
| `fees.write` | listing fee rules |
| `plans.write` | premium plans, benefits, manual subscription grants |
| `users.read` | user search and detail |
| `users.write` | suspend / unsuspend |
| `organisers.review` | approve / reject organisers |
| `events.review` | approve / reject / remove events |
| `payments.read` | listing orders, subscriptions |
| `payments.refund` | refund listing orders |
| `reports.read` | reports queue, moderation history |
| `reports.resolve` | resolve reports, moderation actions |
| `moderation.read_messages` | read messages of a reported conversation (audited) |
| `communities.moderate` | list / remove communities |
| `ads.write` | ad campaigns and stats |
| `notifications.broadcast` | broadcast notifications |
| `location.review` | location identity queue: view proofs (audited), approve / reject / revoke |
| `geo.write` | add / edit districts |

## Rate limits (Redis, `platform/ratelimit`)

| Key | Limit |
|---|---|
| `otp:mobile:<hash>` | `auth.otp.max_per_hour` / hour |
| `otp:ip:<ip>` | `auth.otp.max_per_ip_per_hour` / hour |
| `verify:<userId>` | `verification.max_attempts_per_day` / day |
| `contact:<userId>` | `event.contact_reveals_per_day` / day |
| `chat:<userId>` | `chat.messages_per_minute` / minute |
| `report:<userId>` | `moderation.report_rate_limit_per_hour` / hour |
| `admin-login:<ip>` | `admin.login.max_per_ip` / `admin.login.window_minutes` |
| general `api:<userId>` | 300 / minute (router level, may be added later) |

## Reporting a vulnerability

Email the address in `app.support_email` with "SECURITY" in the subject. Do not open a public issue.

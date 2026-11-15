# auth
Mobile OTP login for app users, rotating refresh tokens and logout. Creates the `users` row
on the first successful verify. Sub-packages `otp/` (OTP challenge store, shared with
guardian) and `tokens/` (refresh tokens, shared with user and admin).

## Routes
See docs/api.md "Auth".

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/auth/otp/request` | Public | |
| POST | `/auth/otp/verify` | Public | |
| POST | `/auth/refresh` | Public | |
| POST | `/auth/logout` | User | |

## Tables
- Owned: `refresh_tokens` (user and admin sessions, via `tokens/`). Inserts `users` rows and
  updates `users.last_login_at`.
- Read: `users` (status), plus everything `user.LoadMe` reads for the login response.
- Redis: `otp:req:<requestId>` (code hash + encrypted mobile), `otp:cooldown:<mobileHash>`,
  rate-limit keys `otp:mobile:<mobileHash>` and `otp:ip:<ip>`.

## Events
- Published: `user.registered` (`UserPayload`) after the first login commits.
- Consumed: none.

## Jobs
- `auth.cleanup_tokens`: deletes refresh tokens expired or revoked more than 7 days ago, in
  batches of 500 (at most 20 batches per run). Safe to re-run.

## Config keys
`auth.otp.length`, `auth.otp.ttl_seconds`, `auth.otp.max_attempts`,
`auth.otp.resend_cooldown_seconds`, `auth.otp.max_per_hour`, `auth.otp.max_per_ip_per_hour`, `auth.access_token_ttl_minutes`,
`auth.refresh_token_ttl_days`. (`auth.sms_provider` is read by the platform SMS selector.)
`DEV_FIXED_OTP` works only when `ENV=local`, and then no SMS is sent.

## Rules
- `otp/request` returns 200 for any valid number and never says whether an account exists.
- Cooldown is checked first, then the hourly limit per mobile and per IP (429 `RATE_LIMITED`).
- Only `SHA-256(requestId:code)` and the encrypted mobile are kept in Redis. Mobiles are looked
  up by `d.Crypto.Hash("mobile", e164)`; raw numbers, codes and tokens are never logged.
- Wrong code: 422 `otp: is incorrect`. After `auth.otp.max_attempts` wrong codes, or when
  expired / used: 422 `otp: has expired, request a new code`. A code works once.
- Suspended users get 403 `ACCOUNT_SUSPENDED` on login and refresh; deleted users get 401.
- Refresh tokens rotate on every refresh (sliding `auth.refresh_token_ttl_days`); reusing a
  rotated token revokes the whole session family.
- Logout with `refreshToken` revokes only that family (and only if it is the caller's); without
  a body it revokes every session of the caller. Always 204.

## Tests
`auth_test.go`: OTP request (422, SMS sent with the configured length, TTL and cooldown from
config, no PII in Redis, cooldown 429, hourly limit per mobile and per IP), `DEV_FIXED_OTP`
local only, verify (wrong, burnt after `max_attempts`, new user with event and refresh TTL from
config, single use, existing user, suspended 403), refresh (rotate, unknown 401, suspended 403,
deleted 401), logout both forms, cleanup job batching. `otp/` and `tokens/` have their own tests.

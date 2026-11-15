# admin
Admin panel login (bcrypt password + TOTP), admin sessions, the admin directory and roles,
the dashboard counts, the audit log viewer, self-service password change and the admin
login audit (`admin_login_events`). Also exports `CreateAdmin` and
`ResetCredentialsByEmail` for `cmd/admincli`.

## Routes
See docs/api.md "Admin auth and admins".

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/admin/auth/login` | AdminPublic | |
| POST | `/admin/auth/refresh` | AdminPublic | |
| POST | `/admin/auth/logout` | Admin | any |
| GET | `/admin/me` | Admin | any |
| POST | `/admin/me/password` | Admin | any |
| GET | `/admin/dashboard` | Admin | `dashboard.read` |
| GET / POST | `/admin/admins` | Admin | `admins.manage` |
| PUT | `/admin/admins/:id` | Admin | `admins.manage` |
| POST | `/admin/admins/:id/reset-credentials` | Admin | `admins.manage` |
| GET / POST | `/admin/roles` | Admin | `admins.manage` |
| PUT | `/admin/roles/:id` | Admin | `admins.manage` |
| GET | `/admin/audit-log` | Admin | `audit.read` |
| GET | `/admin/login-events` | Admin | `audit.read` |

## Tables
- Owned: `admin_users`, `admin_roles`, `admin_login_events` (migration 0010).
- Writes `refresh_tokens` (subject `admin`) only through `auth/tokens`, and `admin_audit_log`
  only through `platform/audit`.
- Read for the dashboard: `users`, `events`, `organiser_profiles`, `reports`,
  `listing_orders`, `subscriptions`, `communities`.

## Events
None published or consumed.

## Jobs
- `admin.purge_login_events`: deletes `admin_login_events` rows older than
  `admin.login_events_retention_days`, 500 rows per batch, at most 20 batches per run.
  Expired admin refresh tokens are removed by `auth.cleanup_tokens`.

## Config keys
`admin.max_failed_logins`, `admin.lockout_minutes`, `admin.access_token_ttl_minutes`,
`admin.refresh_token_ttl_hours`, `admin.login.max_per_ip`, `admin.login.window_minutes`,
`admin.password_min_length`, `admin.login_events_retention_days`.

## Rules
- Login: rate limit `admin-login:<ip>`, `admin.login.max_per_ip` (20) per
  `admin.login.window_minutes` (15) (docs/security.md). Email is matched
  lower-cased. Password (bcrypt cost 12) and TOTP (RFC 6238, 30 s, ±1 step) are both needed.
  Every failure (unknown email, disabled, locked, wrong password, wrong or replayed code)
  returns the same 401; unknown emails still spend one bcrypt compare.
- Each failure adds 1 to `failed_logins`; reaching `admin.max_failed_logins` sets
  `locked_until = now + admin.lockout_minutes` and resets the counter. Success clears both.
- A TOTP code can be used once: `admin:totp:<adminId>:<code>` is set with SETNX for 90 s.
- Refresh rotates the token in its family and keeps the original session expiry (no sliding);
  the new access token carries the role's current permissions. Disabled admins get 401.
- New admins and credential resets get a random password and a new TOTP secret. Only the
  bcrypt hash and the `d.Crypto`-encrypted secret (with `key_version`) are stored; the plain
  values are returned once and never logged.
- You cannot disable yourself. The last active admin with `*` cannot be disabled or moved
  to another role (409). Disabling and resetting revoke the admin's sessions; role changes
  apply on the next refresh.
- Role permissions must come from `Permissions` (docs/security.md). `*` is reserved for the
  seeded `super_admin`, which cannot be edited.
- Login audit: every attempt that passes the rate limit and field checks writes one
  `admin_login_events` row in the login transaction: `admin_id` (NULL for an unknown email),
  `email_hash` = `d.Crypto.Hash("admin_email", lower-cased email)`, IP, user agent (valid
  UTF-8, max 255 characters), `outcome` and a `reason` code (`ok`, `unknown_email`,
  `disabled`, `locked`, `bad_password`, `bad_totp`, `totp_replayed`, `totp_not_set`). The
  reason is never returned to the caller. Passwords, TOTP codes and raw emails are never
  stored. Rate-limited and malformed requests are not recorded (no write flooding).
  `GET /admin/login-events` filters by `adminId`, `email` (hashed server-side) and
  `outcome`, newest first, keyset cursor.
- Change password (`POST /admin/me/password`): needs the current password and a fresh TOTP
  code. Fields are checked first (new password `admin.password_min_length` (clamped 8..72)
  to 72 bytes, no leading/trailing space, different from the current one), so a weak
  password does not burn a code. A wrong password or code counts towards the lockout like a
  failed login and returns 422 `currentPassword` (not 401, so the client keeps its
  session); a locked or disabled account gets 401. Inside the transaction the new password
  must not contain the email name. Success stores the new bcrypt hash, clears the failure
  counter, revokes every refresh token of the admin, issues a new session (returned as
  `{tokens}`) and audits `admin.change_password`. Access tokens already issued to other
  sessions stay valid until they expire (`admin.access_token_ttl_minutes`).
- Every write is audited in the same transaction with ids and flags only
  (`admin.create`, `admin.update`, `admin.reset_credentials`, `role.create`, `role.update`).

## Tests
`admin_test.go`: login happy path with config-driven token TTLs, TOTP replay, wrong password
vs wrong code give the same error, lockout after `admin.max_failed_logins` for
`admin.lockout_minutes`, locked / disabled / unknown accounts, per-IP rate limit, refresh
with current role permissions and fixed expiry, disabled refresh, logout, me, permission
checks on every route, dashboard mapping, create (422, 409, unknown role, encrypted TOTP),
update (self-disable, last super admin, revoke on disable, no-op), reset credentials,
roles (422, `*`, duplicate, super_admin read-only, audit details), audit log filters and
cursor, and the admincli helpers. Every login test also checks the `admin_login_events` row
(HMAC email, outcome, reason).
`security_test.go`: change password (422 fields and policy with no DB, wrong password and
lockout, locked 401, happy path with bcrypt hash, revoke, new session and audit, TOTP replay,
email name), login event IP and user-agent truncation, login events filters and cursor, and
the purge job batches and cutoff.

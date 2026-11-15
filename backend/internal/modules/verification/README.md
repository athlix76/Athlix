# verification
Identity and age verification through DigiLocker: direct OAuth2 + PKCE, a KYC aggregator's
hosted DigiLocker session, or a mock provider outside prod. It sets the user's verification
status and minor flag, advances onboarding, and maps the Aadhaar address to a native district
for location eligibility (the address itself is never kept).

## Routes
See docs/api.md "Me, onboarding, verification" and docs/flows/signup-digilocker.md.

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/verification/digilocker/start` | User | |
| GET | `/verification/digilocker/callback` | Public (one-time `state`) | |
| GET | `/verification/status` | User | |
| POST | `/verification/mock/complete` | User | 404 unless `verification.provider` = `mock` and ENV is not prod |

## Tables
- Owned: `identity_verifications`.
- Writes through the onboarding helpers: `users.verification_status`, `users.is_minor`,
  `users.onboarding_step` (via `user.Advance`), `profiles.birth_year` (when a profile row exists).
- Redis: `dl:state:<sha256(state)>` (pending attempt, TTL `verification.state_ttl_seconds`) and
  the rate-limit counter `rl:verify:<userId>`.

## Events
- Published: `verification.completed` (`events.VerificationPayload`) after every finished attempt
  commits. `status` is `verified` or `failed`; `isMinor` and `nativeDistrictId` are set only when
  verified (`nativeDistrictId` is 0 when the address district does not match).
- Consumed: `user.deleted`: overwrites name, DOB, gender and last 4 with NULL (idempotent).
  `subject_hash` stays so one identity still maps to one account.

## Jobs
- `verification.minor_recheck`: in batches, decrypts the verified DOB of every minor; anyone who
  has reached `guardian.minor_age_years` gets `is_minor = 0` and their onboarding step recomputed.
- `verification.purge_subject_hash`: for users with `status = 'deleted'` and `deleted_at` older
  than `verification.subject_hash_retention_days` (180), sets `subject_hash` and `provider_ref`
  to NULL on their attempts (500 rows per batch, at most 20 batches per run; the UPDATE re-checks
  the user is still deleted). Code in `jobs.go`.

## Config keys
`verification.provider`, `verification.max_attempts_per_day`, `verification.state_ttl_seconds`,
`verification.name_match_required`, `verification.subject_hash_retention_days`, `auth.min_age_years`,
`guardian.minor_age_years`.
Secrets: `digilocker` (`DigiLockerSecret`) and `kyc-aggregator` (`AggregatorSecret`), read through
`d.Secrets` with `d.Cfg.SecretName`.

## Rules
- The server does the code exchange. The PKCE verifier and the attempt id live only in Redis,
  keyed by a hash of `state`; `state` is redeemed with GETDEL, so it works once. An unknown, used
  or expired state redirects with `reason=provider_error` and touches nothing.
- Each start or mock call counts against `verification.max_attempts_per_day` (429).
- Every write locks the attempt with `WHERE id = ? AND user_id = ?` and needs it still
  `initiated` and the user not yet verified; anything else is a no-op.
- Age is computed in IST on the current day. Below `auth.min_age_years` → `failed/too_young` with
  no identity data stored. Below `guardian.minor_age_years` → `isMinor`, step `guardian`, and
  `guardian_consent_status` moves from `not_required` to `pending`. When the recheck job clears
  `is_minor`, consent goes back to `not_required` (a `revoked` consent is kept).
- `verification.name_match_required`: every word of the display name (if set) must appear in the
  verified name; a single letter matches an initial. Otherwise `failed/name_mismatch`.
- Subject id → `d.Crypto.Hash("identity_subject", ...)`. Held by another user, or a duplicate-key
  race on the unique index → `failed/already_linked`.
- Kept: provider, provider ref, last 4, `name_enc`, `dob_enc` (`YYYY-MM-DD`), gender, key version,
  subject hash. Thrown away: full Aadhaar number, XML, photo, access token, address. Provider
  bodies, tokens and identity fields are never logged or returned; logs carry ids only.
- The address state and district are matched with `d.Eligibility.Places().MatchDistrict` (name
  or alias), then cleared. Mock calls take an optional `nativeDistrictId` instead, checked with
  `Places().District` (unknown → 422).
- De-dup vs retention: `subject_hash` is only cleared for accounts deleted longer than
  `verification.subject_hash_retention_days`. Active and suspended accounts keep it, so one
  identity still maps to at most one live account (the unique index allows many NULLs). Within
  the retention a deleted user's identity gets `failed/already_linked` on a new account
  (anti-fraud / ban evasion); after it the identity can verify again on a new account.
- `BirthYear(ctx, d, q, userID)` is exported for the profile module (birth year of the verified
  DOB).

## Tests
`verification_test.go`, with loopback httptest servers standing in for DigiLocker and the
aggregator. It covers:
- start (state hashed, TTL, PKCE challenge, session ref), plus every reason start is refused
  (409, 404, 502, 429);
- the direct callback: adult, the "Tanjore" alias → 26, the minor path, too_young, the
  configurable age limits, already_linked, the duplicate-key race, the name rule, cancel,
  provider errors, bad XML, and single-use / unknown state;
- an attempt that does not belong to the state's user;
- the aggregator: no match → 0, alias match, cancel;
- mock: pass-through of the district, every outcome, 422 on an unknown district or bad outcome,
  404 in prod or when the provider is not mock, and the rate limit;
- status, the minor recheck job, idempotent wipe on `user.deleted`, `BirthYear`, IST age, name
  matching, and parsing (XML, DOB, gender).

`purge_test.go`: the purge job selects only deleted users past
`verification.subject_hash_retention_days`, clears in batches of 500 with a bound IN list, stops
on a short batch, and uses the 180-day default.

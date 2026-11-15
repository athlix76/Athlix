# guardian
Guardian consent for verified minors. The minor names a guardian; the guardian gets an OTP by
SMS; entering it grants consent and moves onboarding past the `guardian` step. Guardian name and
mobile are stored encrypted and wiped when the minor's account is deleted.

## Routes
See docs/api.md "Me, onboarding, verification" and docs/flows/minor-consent.md.

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/onboarding/guardian/request` | User | caller must be verified and a minor |
| POST | `/onboarding/guardian/verify` | User | consent must belong to the caller |
| POST | `/me/guardian/withdraw/request` | User | caller is a minor with a `granted` consent; code goes to the guardian's mobile |
| POST | `/me/guardian/withdraw/verify` | User | consent must belong to the caller; revokes it, step back to `guardian` |
| POST | `/admin/users/:id/guardian-consent/revoke` | Admin | `users.write`; `{reason}`, audited (`guardian.consent_revoke`) |

## Tables
- Owned: `guardian_consents`.
- Writes through the onboarding helpers: `users.guardian_consent_status`, `users.onboarding_step`
  (via `user.Advance`).
- Read: `users` (verification status, minor flag, `mobile_hash`), `policy_documents`
  (active `guardian_consent` document).
- Redis: `guardian:otp:<consentId>` (code hash + owner id, TTL `guardian.consent_otp_ttl_seconds`),
  `guardian:cooldown:<userId>`, and rate-limit counters `rl:guardian:user:<userId>` /
  `rl:guardian:mobile:<hex mobile hash>`.

## Events
- Published: `guardian.consent_granted` (`events.UserPayload`) after the grant commits.
- Published: `guardian.consent_revoked` (`events.UserPayload`) after a withdrawal or admin revoke
  commits (`revoked_reason` = `guardian_withdrew` or the admin's reason).
- Consumed: `user.deleted`: overwrites guardian name and mobile with empty ciphertext, the hash with
  random bytes and last 4 with `0000`; a pending consent becomes `expired`. Idempotent; a
  malformed payload is dropped.

## Jobs
None.

## Config keys
`guardian.consent_otp_ttl_seconds`, `auth.otp.resend_cooldown_seconds`, `auth.otp.max_per_hour`,
`auth.otp.length`, `auth.otp.max_attempts` (the OTP limits are shared with login OTPs).

## Rules
- Request: name 2-80 characters with no control characters, a valid Indian mobile, relation one of
  `mother`, `father`, `legal_guardian`, `other`, and an active `guardian_consent` policy document
  (422 otherwise). The caller must be verified, a minor, and not yet granted (409).
- The guardian mobile may not be the minor's own number (compared by `d.Crypto.Hash("mobile")`).
- Resend cooldown → 429; at most `auth.otp.max_per_hour` requests per hour per minor and per
  guardian mobile → 429.
- A new request expires any earlier pending consent of the same minor.
- Name and mobile go through `d.Crypto` (`_enc`, `_hash`, `_last4`). Responses show only
  `******<last4>`. No name, number or OTP is logged; Redis holds only the code hash.
- Verify: the consent is loaded with `WHERE id = ? AND user_id = ?` first, so another user's
  consent id → 404 and does not use up the challenge. Not pending → 409. Wrong code → 422
  "is incorrect"; after `auth.otp.max_attempts` wrong codes, or after the TTL → 422 "has expired".
- The grant re-locks the consent row and still needs it `pending`.
- `ENV=local` with `DEV_FIXED_OTP` uses the fixed code and sends no SMS. SMS failure → 502.

## Tests
`guardian_test.go`: the request happy path (SQL order, ciphertext, hash, masked mobile, SMS
template and number, OTP length and TTL from config, no raw data in Redis); the cooldown and hourly
limit; validation; own mobile; inactive policy; the state checks (adult, unverified, already
granted); SMS failure; the verify happy path with the event and single use; wrong and burnt codes;
another user's consent (404, challenge kept); verify validation; the fixed dev OTP; and the
idempotent `user.deleted` wipe.

# user
The signed-in user: `Me`, settings, policy acceptance, roles, the onboarding step machine
(`Advance` / `NextStep`, shared with auth, verification, guardian and profile) and account
deletion. Also the admin user list, detail, suspend and unsuspend.

## Routes
See docs/api.md "Me, onboarding, verification" and "Users and organisers".

| Method | Path | Group | Permission |
|---|---|---|---|
| GET | `/me` | User | |
| DELETE | `/me` | User | |
| GET / PUT | `/me/settings` | User | |
| POST | `/me/policies/accept` | User | |
| POST | `/onboarding/role` | User | |
| POST | `/me/roles` | Verified | |
| GET | `/admin/users` | Admin | `users.read` |
| GET | `/admin/users/:id` | Admin | `users.read` (view audited `user.view`) |
| POST | `/admin/users/:id/suspend` | Admin | `users.write` |
| POST | `/admin/users/:id/unsuspend` | Admin | `users.write` |

## Tables
- Owned: `users` (row created by auth on first login; this module and the onboarding helpers
  change it), `user_roles`, `user_settings`, `policy_acceptances`.
- Writes `refresh_tokens` only through `auth/tokens` (revoke on delete / suspend).
- Read: `profiles` (photo, has-profile), `policy_documents`, `media_uploads` (via `d.Media`),
  `identity_verifications`, `guardian_consents`, and for admin counts `events`,
  `event_registrations`, `posts`, `reports`.

## Events
- Published: `events.UserDeleted` (`user.deleted`, `events.UserPayload`) after the
  `DELETE /me` transaction commits, only when a row changed.
- Consumed: none.

## Jobs
None.

## Config keys
None directly. `NextStep` reads `verification_requirements` through `d.Requirements`
(gate `app_access`: `digilocker`, `guardian_consent_if_minor`).

## Rules
- Steps: `role` → `verification` (if DigiLocker is required for any of the user's roles) →
  `guardian` (minor and consent required) → `profile` → `done`. `done` never goes back.
  `Advance` must be called in the same transaction as the state change (it locks the row).
- `/onboarding/role` only while the step is `role` (else 409 `INVALID_STATE`).
- Policy ids must be active documents (else 422). Acceptance is idempotent.
- `DELETE /me`: one transaction wipes `mobile_enc`, sets a random `mobile_hash` (the number can
  sign up again as a new user), clears the display name and settings, revokes every session.
  Always 204; the event is published only when a row changed.
- Admin search: `#42` = id, a valid Indian mobile = HMAC hash lookup (the number is never
  stored or logged), otherwise a display-name prefix (LIKE wildcards escaped, bound arg).
- Suspend needs a reason (1-255 chars), revokes every session, is audited in the same
  transaction (audit details hold no free text). Deleted users are 404.
- Mobile numbers are only ever shown masked (`******1234`). The verified name is decrypted only
  in the audited admin detail view.

## Tests
`user_test.go`: Me (null optionals, store token), deleted user 404, settings defaults / partial
merge / 422, policy acceptance (empty, inactive, idempotent), add role, onboarding role (422,
409, happy path through `Advance`), `NextStep` with requirement rows switched on and off,
`done` never regresses, delete (event once, idempotent), admin permission checks, list filters
(id, mobile hash, escaped name prefix, cursor), audited detail, suspend / unsuspend states.
`usertest/` holds the shared sqlmock helpers.

# locationidentity

Native and residence location identities: the user's proof submission, the admin review queue,
expiry and proof deletion, and the `states` / `districts` reference tables. The eligibility rule
itself lives in `platform/eligibility`, which reads the rows this module writes.

## Routes

See [docs/api.md](../../../../docs/api.md) sections "Location identity" and "Admin".

| Method | Path | Group | Permission |
|---|---|---|---|
| GET | `/geo/states` | user | |
| GET | `/geo/districts?stateId` | user | |
| GET | `/me/location-identities` | verified | |
| POST | `/me/location-identities` | verified | |
| DELETE | `/me/location-identities/:id` | verified (owner) | |
| GET | `/admin/location-identities?status&identityType&stateId&districtId&cursor&limit` | admin | `location.review` |
| GET | `/admin/location-identities/:id` | admin | `location.review` |
| POST | `/admin/location-identities/:id/approve` | admin | `location.review` |
| POST | `/admin/location-identities/:id/reject` | admin | `location.review` |
| POST | `/admin/location-identities/:id/revoke` | admin | `location.review` |
| GET | `/admin/geo/states` | admin | `config.read` |
| GET | `/admin/geo/districts?stateId&q&includeInactive` | admin | `config.read` |
| POST | `/admin/geo/districts` | admin | `geo.write` |
| PUT | `/admin/geo/districts/:id` | admin | `geo.write` |

`ImportDistricts` (`importer.go`) is the CSV importer behind `cmd/admincli import-districts`.

## Tables

- Owned: `user_location_identities`, `states`, `districts`.
- Written as a documented cross-module exception: `policy_acceptances` (one row for the active
  notice, `INSERT IGNORE`, in the POST transaction).
- Written through platform packages: `media_uploads` (`platform/media` Claim / Purge),
  `admin_audit_log` and `config_audit` (`platform/audit`).
- Read: `policy_documents` (active `location_identity_notice`), `users` (`is_minor`), `profiles`
  (`district`, shown to the reviewer), `admin_users` (reviewer name), user cards through
  `platform/userdir`.

## Events

- Published: `location_identity.changed` after every committed status change, one per changed
  row (pending, verified, superseded, rejected, revoked, withdrawn, expired);
  `location_identity.expiring` from the reminder job.
- Consumed: `verification.completed` (status `verified` and `nativeDistrictId > 0` → create a
  verified DigiLocker native row if the user has none), `user.deleted` (withdraw every pending /
  verified row, purge every location proof of the user). Both are idempotent.

## Jobs

| Job | What it does (at most 500 rows per run, safe to re-run) |
|---|---|
| `location.expire_residence` | verified residence with `expires_at <= now` → `expired`, publish changed |
| `location.expiry_reminders` | verified residence expiring within `location.expiry_reminder_days`, not yet reminded → set `expiry_reminded_at`, publish expiring (0 = off) |
| `location.purge_proofs` | rows not pending whose `reviewed_at` (else `updated_at`) is older than `location.proof_retention_days`, and withdrawn rows, with a proof → `media.Purge`, set `proof_purged_at`, `proof_upload_id = NULL` |

## Config keys

| Key | Default | Use |
|---|---|---|
| `location.enabled` | `true` | off → POST returns 409 `INVALID_STATE`, `canSubmit` is false |
| `location.native_proof_types` | `["masked_aadhaar","birth_certificate"]` | accepted native proofs |
| `location.residence_proof_types` | `["rent_agreement","utility_bill","bank_statement","voter_id","employer_letter","institution_id"]` | accepted residence proofs |
| `location.native_change_allowed` | `true` | off → a user with a verified native cannot submit another (409 `INVALID_STATE`) |
| `location.residence_change_cooldown_days` | `180` | days after `verified_at` before a different residence district may be submitted |
| `location.residence_validity_months` | `12` | residence `expires_at` set at approval |
| `location.minor_guardian_proof_allowed` | `true` | minors may submit a guardian proof for residence |
| `location.proof_retention_days` | `30` | purge job cutoff |
| `location.expiry_reminder_days` | `30` | reminder window |
| `location.utility_bill_max_age_days` | `90` | reviewer checklist value |
| `location.rejection_reasons` | see docs/config-system.md | allowed `rejectionCode` values |
| `location.proof_view_url_ttl_seconds` | `120` | read by `platform/media.ReviewerViewURL` |

## Rules

- The caller comes from `authn` only. Ownership is in the SQL (`WHERE id = ? AND user_id = ?`);
  someone else's row is 404.
- POST: `proofType` must be in the list for the identity type; `districtId` must be active and
  `stateId` is taken from the district; `noticePolicyDocumentId` must be the active
  `location_identity_notice` (its version is stored in `notice_version`); the upload is claimed
  with purpose `location_proof` in the same transaction and may back only one row. The same
  transaction records the notice acceptance in `policy_acceptances` (`INSERT IGNORE`, so a user
  who already accepted this notice version keeps one row).
- One pending row per type: a pending row gives 409 `CONFLICT`, and the `pending_slot` unique
  key violation maps to the same error.
- Residence change to a different district within the cooldown → 409 `LIMIT_REACHED` with
  `fields.nextChangeAt`. A renewal for the same district is always allowed.
- A guardian proof is only for a minor's residence and only when the setting allows it.
- Approve supersedes the user's old verified row of the same type before verifying the new one,
  in one transaction (the `verified_slot` unique key never sees two). `aadhaarLast4` is accepted
  only for `masked_aadhaar` and must be exactly 4 digits. The optional `note` (trimmed, at most
  255 characters, else 422 field `note`) is stored in `review_note` and returned only in the admin
  detail as `reviewNote`; the audit row records `withNote`, not the text.
- Reject: `rejectionCode` must be in `location.rejection_reasons`; `unmasked_aadhaar` deletes
  the proof right after commit. Revoke: verified rows only; the reason is kept in
  `rejection_reason`.
- Withdraw: pending or verified → withdrawn; the proof is deleted right after commit (the purge
  job retries if that fails).
- Every admin write adds an `admin_audit_log` row in the same transaction. Signing a proof URL
  writes `location.proof_viewed` first; no audit row, no URL. District create / update also
  write `config_audit` and call `Places.Bump` after commit: this instance reloads at once and the
  Redis counter `places:version` makes the others reload within 15 s. A Redis error is logged and
  never fails the request; the cache TTL still applies.
- Events are published after commit only. Logs carry ids only: never district names together
  with user ids, upload keys, `aadhaarLast4` or rejection notes.
- All SQL uses bind parameters; the admin district search escapes `LIKE` wildcards.

## Tests

`user_test.go`: states / districts, displayStatus / expiresInDays / nextChangeAt, POST happy path,
422 for proof type (default and config list), guardian rules, wrong notice and inactive district,
notice acceptance row written in the POST transaction, cooldown 409 with `nextChangeAt`, cooldown from config, same-district renewal, pending exists and
duplicate key 409, native change disabled / allowed, withdraw with proof purge, ownership 404.
`admin_test.go`: 403 without permission, queue, detail signs proof and audits, approve
supersedes and sets residence expiry from config, `aadhaarLast4` rules, approve note stored and returned / too long 422, reject `unmasked_aadhaar`
purges, rejection code from config, revoke only verified, district list / create / update with
audit, admin states list. `jobs_test.go`: `verification.completed` idempotent and no-op without a district,
`user.deleted`, each job with a config change. `importer_test.go`: header, upsert by lgd_code and
by name, aliases, unknown state and bad rows skipped, rollback on a database error.

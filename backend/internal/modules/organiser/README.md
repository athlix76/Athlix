# organiser
Organiser verification: an organiser fills the checklist from `organiser_verification_steps`
(text, document, checkbox), submits it, and an admin with `organisers.review` approves or
rejects it. Approval is what the `organiser_approval` requirement checks before event creation.

## Routes
See docs/api.md "Organiser verification" and "Users and organisers".

| Method | Path | Group | Permission |
|---|---|---|---|
| GET | `/organiser/verification` | Verified | role `organiser` (else 403 `FORBIDDEN`) |
| PUT | `/organiser/verification` | Verified | role `organiser`; status `not_started`, `draft` or `rejected` |
| POST | `/organiser/verification/submit` | Verified | role `organiser`; status `not_started`, `draft` or `rejected` |
| GET | `/admin/organisers` | Admin | `organisers.review` |
| GET | `/admin/organisers/:userId` | Admin | `organisers.review` (view audited) |
| POST | `/admin/organisers/:userId/approve` | Admin | `organisers.review` |
| POST | `/admin/organisers/:userId/reject` | Admin | `organisers.review` |

## Tables
- Owned: `organiser_profiles`, `organiser_verification_submissions`.
- Read: `organiser_verification_steps` (owned by appconfig), `users` / `user_roles` and user cards
  (through `d.Users`), `media_uploads` (through `d.Media`).
- Writes `admin_audit_log` (through `audit.Admin`) on every admin decision and detail view.

## Events
- Published after the commit: `organiser.approved` and `organiser.rejected`
  (`events.OrganiserPayload{UserID, Reason}`; `Reason` is the rejection note).
- Consumed: `user.deleted`: purges every organiser document, deletes the submissions and clears
  the organisation name and review note. Idempotent; a malformed payload is dropped; DB errors
  are returned so the event is retried.

## Jobs
None.

## Config keys
None of its own. The checklist is the `organiser_verification_steps` table; document size and
types come from `media.document_max_mb` / `media.allowed_document_types` (via `d.Media`).

## Rules
- The caller is always `authn.UserID(c)`; organisers only ever see their own row. Admin routes
  take `:userId`; an unknown, deleted, or never-started user → 404.
- PUT: a missing field means unchanged; an empty `orgName` clears it. `orgName` at most 120
  characters with no control characters, `orgType` from the enum. Each step entry needs an active
  `stepId` (no duplicates) and the field of its input type: `valueText` (at most 500, empty
  clears), `documentUploadId` (0 clears; must be the caller's own `organiser_document` upload,
  checked by `d.Media.Claim`), or `checked` (false clears; stored as `value_text = 'true'`).
  Saving sets status `draft`; a replaced or cleared document is purged after the commit.
- Submit needs `orgName`, `orgType` and every active required step filled (422 with
  `steps.<stepCode>` fields). It sets `submitted_at` and resets every submission to `submitted`.
- Approve (`note` optional) and reject (`note` required, at most 500) need status `submitted`
  (409 otherwise). Both store the note in `organiser_profiles.review_note` (NULL when absent), and
  it is returned as `reviewNote` in the admin detail and in the organiser's own GET; an approve
  note is therefore visible to the organiser, so reviewers write it for them. They set `reviewed_by` / `reviewed_at` on the profile and every submission and
  write `organiser.approve` / `organiser.reject` audit rows (details: `withNote` only) in the
  same transaction.
- The steps shown are the active ones plus any inactive step the user already filled (never
  required). Document links are short-lived signed URLs, signed only after the caller check.
- Every query uses bind parameters; logs carry user and admin ids only.

## Tests
`organiser_test.go`: the role gate (403) on the three app routes; GET when not started (nulls
present) and filled (text, document with a signed URL, checkbox, inactive step); the save happy
path (SQL order, claim, trimmed values); unchanged fields, clearing, document replace and purge
after commit; validation before and after loading the steps; another user's, wrong-purpose and
not-uploaded documents (422); 409 outside the editable statuses; rollback with no purge; submit
missing fields and the happy path from draft and rejected; admin permission (403) on every admin
route; list paging, status filter and skipped cards; detail with audit and 404s; approve with and
without a note, a repeat approve (409, no second event), reject validation and payload, 404s, and
audit failure rolling back with no event; the idempotent `user.deleted` wipe.

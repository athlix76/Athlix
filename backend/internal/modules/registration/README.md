# registration

Players register for published events (team name and up to 30 member names), withdraw, and
follow their registrations. Organisers see and manage the registrations on their events. Location
eligibility is enforced inside the registration transaction and re-checked when identities change.

## Routes

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/events/:id/registrations` | Verified | not the organiser |
| DELETE | `/events/:id/registrations/me` | Verified | own registration |
| GET | `/me/registrations` | Verified | own rows (`?status`) |
| GET | `/events/:id/registrations` | Verified | event owner (`?status&flagged`) |
| GET | `/admin/events/:id/registrations` | Admin | `events.review`; organiser view of any event (`?status&flagged`) |
| PATCH | `/events/:id/registrations/:regId` | Verified | event owner |

## Tables

- Owned: `event_registrations`.
- Writes by exception: `events.registrations_count` (atomic `GREATEST(count + ?, 0)` under the
  event row's `FOR UPDATE`).
- Read: `events`, `profiles.birth_year`, `identity_verifications.gender`, `blocks`, `users`
  (through `d.Users`), `user_location_identities` (through `d.Eligibility.Load` / `Check`, and
  `id, status` for `qualifiedLabel`). `GET /me/registrations` renders `event` with
  `event.Summaries` (same owner).

## Events

- Published (after commit): `registration.created`, `registration.withdrawn` (player withdraw,
  and organiser reject with `status: "rejected"`), `registration.promoted`,
  `registration.payment_marked`, `registration.eligibility_flagged` (with `eligibilityFlag`).
- Consumed: `location_identity.changed`.

## Jobs

None.

## Config keys

`registration.withdraw_until_hours` (24), `registration.waitlist_enabled` (true),
`location.flag_registrations_on_change` (true), `location.enabled` (through `d.Eligibility`).

## Rules

- Register: event `published` (draft / review / rejected / removed → 404, cancelled /
  completed → 409 `INVALID_STATE`), not the organiser (403), not blocked either way (404),
  deadline in the future (409 `INVALID_STATE`), age from `profiles.birth_year` at the event's
  IST start year and gender `M`/`F` against `male`/`female` (unknown → allowed; mismatch → 403
  `NOT_ELIGIBLE` with `fields.reason` = `age_group` | `gender_category`, `fields.action` =
  `none`, and a specific message).
- Location: non-open rules run `d.Eligibility.Check(ctx, tx, …)` in the same transaction; not
  eligible → 403 `NOT_ELIGIBLE {reason, action}`. The qualifying identity is stored
  (`qualified_via`, `qualified_identity_id`, `qualified_district_id`). Open events never load
  identities; `location.enabled = false` registers as `open`.
- A second registration → 409 `CONFLICT` (also on the UNIQUE race). A withdrawn row is
  reused (re-registration). Full → `waitlisted` when the waitlist is on, else 409 `LIMIT_REACHED`.
- Withdraw until `registration.withdraw_until_hours` before start (409 `INVALID_STATE`). A freed
  slot promotes the oldest waitlisted player who still passes `Check`; players who fail are
  skipped and flagged (flag from the qualifying identity's status, else `identity_expired` /
  `identity_changed`). Promotion stops once the event has started.
- `location_identity.changed` with a status other than `verified` / `pending`: every
  `registered` / `waitlisted` row on published, not-started events whose
  `qualified_identity_id` is that identity is re-evaluated with the user's identities loaded in
  the same tx. Still eligible via another identity → `qualified*` updated and the flag cleared;
  otherwise `eligibility_flag` (expired → `identity_expired`, revoked → `identity_revoked`,
  rejected → `identity_rejected`, withdrawn / superseded → `identity_changed`) and
  `eligibility_flagged_at` are set and `registration.eligibility_flagged` is published. Never
  cancels. Idempotent (`WHERE eligibility_flag <> ?`; requalified rows drop out). Batches of 500.
- Organiser view adds `user` and `qualifiedLabel` (`Native: Thanjavur (verified)`,
  `Residence: Coimbatore (in review)`, `(expired)`, `(no longer valid)`, or `Open`).
- PATCH: `paymentStatus` enum, `paymentNote` ≤ 200 (blank clears), `status` only `rejected`
  (withdrawn → 409). Ownership is the event's `organiser_id` vs the token user and the
  registration is loaded `WHERE id = ? AND event_id = ?`; any mismatch → 404.

## Tests

`registration_test.go`: register (open happy path + event, eligibility table: eligible native,
eligible residence, wrong place, missing, expired proof, pending under require_verified
true/false, either, state scope, switch off), rules (own event 403, missing / draft 404,
cancelled 409, blocked 404, deadline 409, age 403 `NOT_ELIGIBLE` `age_group`, unknown facts allowed,
gender 403 `NOT_ELIGIBLE` `gender_category`, second
409, reactivation, waitlist, no-waitlist 409, duplicate race 409), validation 422, withdraw
(promotion re-check skips + flags, waitlisted withdraw, window 409 for the setting and default,
404), organiser list (404 for non-owner, 422, `flagged` filter, labels), patch (422, 404 owner,
404 foreign registration, payment_marked, reject + promotion, withdrawn 409), `/me/registrations`
with EventSummary, and `location_identity.changed` (requalify vs flag, re-delivery idempotent,
flag mapping, skips for verified/pending/settings off, batching).

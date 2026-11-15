# event

Organisers create and run events (draft → listing payment → review → published → completed).
Players browse a feed filtered by location eligibility, distance and profile facts, and reveal
the organiser contact. Admins review, reject and remove events.

## Routes

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/events` | Verified | role `organiser` (+ approved profile when required) |
| GET | `/events` | Verified | |
| GET | `/events/featured` | Verified | |
| GET | `/events/eligibility/preview` | Verified | |
| GET | `/events/:id` | Verified | owner for non-published |
| PUT | `/events/:id` | Verified | owner |
| GET | `/events/:id/contact` | Verified | |
| POST | `/events/:id/cancel` | Verified | owner |
| POST | `/events/:id/feature` | Verified | owner + benefit `featured_events` |
| GET | `/me/events` | Verified | |
| GET | `/admin/events` | Admin | `events.review` |
| GET | `/admin/events/:id` | Admin | `events.review` |
| POST | `/admin/events/:id/approve` | Admin | `events.review` |
| POST | `/admin/events/:id/reject` | Admin | `events.review` |
| POST | `/admin/events/:id/remove` | Admin | `events.review` |
| POST | `/admin/events/:id/feature` | Admin | `events.review`; published events only, repeat is a no-op |
| POST | `/admin/events/:id/unfeature` | Admin | `events.review` |

## Tables

- Owned: `events`.
- Read: `sports`, `organiser_profiles`, `profiles.birth_year`, `identity_verifications.gender`,
  `blocks`, `users`, `event_registrations` (my registration), `conversations` (group chat flag),
  `listing_orders` / `listing_fee_rules` (owner and admin listing view),
  `user_location_identities` (through `d.Eligibility.Load`), `subscriptions` (through `d.Entitlements`).

## Events

- Published: `event.updated` (published or pending_payment edit), `event.cancelled` (owner
  cancel, admin remove), `event.published` (admin approve), `event.rejected`, `event.completed`.
- Consumed: none.

## Jobs

- `event.complete_past` (hourly): published events whose `ends_at` passed → `completed`,
  batches of 500, guarded by status so re-runs are no-ops.

## Config keys

`event.require_admin_approval` (read by listingpayment), `event.show_contact`,
`event.max_active_per_organiser`, `event.min_hours_before_start`, `event.allow_free_events`,
`event.max_slots_limit`, `event.contact_reveals_per_day`, `event.featured_list_size`,
`event.max_search_radius_km`, `event.eligibility_default_government`,
`event.eligibility_default_private`, `location.enabled` (through `d.Eligibility`).

## Rules

- Location eligibility uses only `platform/eligibility`: `Rule.Validate`, `Evaluate`,
  `Filter`, `Describe`, `Tag`, `Places()`. The feed condition is `eligibility.Filter` over the
  identities loaded once per request (the same rows `myEligibility` uses). Open events and the
  switch off (`location.enabled = false`) never query identities.
- District scope derives `eligibility_state_id` from the district; a client `stateId` is ignored.
  Inactive districts / unknown states → 422 `eligibility.*`.
- `category`, `eligibility`, fee inputs, dates and the other non-cosmetic fields are locked once
  published (422 "is locked once the event is published"). `pending_review` edits → 409.
  A `pending_payment` edit returns the event to `draft` and emits `event.updated`
  (listingpayment expires the open order).
- Non-owners who are not eligible get `paymentInstructions: null`, and `/contact` returns
  403 `NOT_ELIGIBLE {reason, action}` (checked again right before the reveal).
- Contact reveals are rate-limited per `contact:<userId>` per 24 h; the owner is exempt.
- Featuring counts `featured_at` in the current IST calendar month against the
  `featured_events` benefit (0 → 403 `PLAN_REQUIRED`, used up → 409 `LIMIT_REACHED`).
- `lat/lng` are bound into a haversine expression and never stored or logged; `radiusKm`
  1..`event.max_search_radius_km`; events without coordinates are excluded when a radius or
  `sort=distance` is given.
- The contact phone is stored encrypted (`d.Crypto`) with its last 4 digits for masking.
- Non-published events are visible to the owner only (cancelled also to registered players);
  blocked either way → 404.

## Tests

`event_test.go`: create (happy, phone encrypted, role, organiser approval on/off, active
limit, validation, district → state derivation, bad eligibility), update (ownership 404,
pending_review 409, published locks, allowed edit emits `event.updated`, pending_payment →
draft), get visibility, eligibility scenarios (native, residence, wrong place, missing, expired
by status and by date, pending with require_verified true/false, either, state scope, switch
off, open never loads), list (feed filter, showAll greys, SQL builder binds coordinates and
terms, radius / sort validation, distance rounding, featured), preview, contact (needs
registration, NOT_ELIGIBLE, reveal + 429, owner), cancel, feature (plan, monthly limit,
happy + idempotent, draft), admin (403, approve, wrong state, reject, remove, list) and the
completion job.

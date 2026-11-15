# search

People search and the global search box. Read-only: it reads users, profiles, events and
communities owned by other modules and writes nothing.

## Routes

See [docs/api.md](../../../../docs/api.md) section "Search".

| Method | Path | Group | Permission |
|---|---|---|---|
| GET | `/search/people?q&role&sportId&positionId&state&district&city&ageMin&ageMax&achievementLevel&cursor&limit` | verified | |
| GET | `/search?q` | verified | |

## Tables

- Owned: none.
- Read: `users`, `profiles`, `user_settings` (`privacy.showInSearch`), `user_roles`, `blocks`,
  `follows`, `achievements`, `subscriptions` + `premium_plan_benefits` (`profile_boost`), `events`,
  `sports`, `conversations`, `conversation_members`, `media_uploads` (through `platform/media`),
  `user_location_identities` (through `d.Eligibility.FeedFilter`).

## Events

None published or consumed.

## Jobs

None.

## Config keys

| Key | Default | Use |
|---|---|---|
| `search.page_size` | `20` | default page size of `/search/people` (`?limit` may override, max 100) |
| `minor.searchable` | `true` | off → minors never appear in either search |
| `location.enabled` | `true` | off → no location-eligibility filter on event results (read by `platform/eligibility`) |

## Rules

- Excluded from people results: the caller, users blocked either way, `status <> 'active'`,
  `onboarding_step <> 'done'`, and `privacy.showInSearch = false` (no settings row = shown).
- Minors are shown without `photoUrl`, `city` and `bio` unless the caller and the minor follow
  each other. Filters on city, district and the bio part of `q` match a minor only for a mutual
  follow, so a filter cannot reveal a hidden field.
- `ageMin`, `ageMax` and `achievementLevel` need the `advanced_search_filters` benefit, else 403
  `PLAN_REQUIRED` with `fields.benefit`. Age uses `profiles.birth_year` and the IST year; users
  without a birth year do not match an age filter.
- Order: users with an active `profile_boost` benefit first, then newest id. Keyset cursor on
  (boosted, id).
- `q` matches display name, academy / club and bio with bound `LIKE`; `%`, `_` and `\` in the
  term are escaped. All values are bind parameters.
- `/search`: `q` required, 2–100 characters (trimmed) → else 422. Returns up to 5 people (same
  rules), 5 published events (title / venue / city; organiser active and not blocked either way;
  soonest first) and 5 public active communities (title / description; owner not blocked either
  way; largest first). `myStatus` is the caller's membership status or `none`; `myRole` only when
  active. Cards and media URLs are loaded in one call each.
- Event results are limited to events the caller is location-eligible for:
  `d.Eligibility.FeedFilter(ctx, "e", caller, now)` is ANDed into the events query (open events,
  plus native / residence / either events matching a usable identity; pending proofs only match
  `requireVerified = false`; expired proofs never match). The rule lives only in
  `platform/eligibility`; with `location.enabled = false` the condition is `1=1`. There is no
  `showAll` on `/search`.

## Tests

`search_test.go`: filter args and exclusion SQL, boosted order, minor fields stripped vs kept for
a mutual follow, `minor.searchable=false`, advanced filters → `PLAN_REQUIRED` and allowed with
the benefit (birth-year args), validation 422s, page size from config and cursor round trip,
global result shape (slotsLeft, isFeatured, SportRef code, banner / avatar URLs, myStatus /
myRole), empty arrays, `q` validation, LIKE escaping; location eligibility on events: no identity
(open only), verified native, verified residence, native + residence (either), pending proof
(requireVerified = 0 only), expired proof ignored, switch off (`1=1`).

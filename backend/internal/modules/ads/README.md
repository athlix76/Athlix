# ads

Contextual sponsored placements. The app asks for ads per placement (plus optional sport and
state context); admins manage campaigns and read daily impression / click counters. No user
data is ever stored or used for targeting.

## Routes

See [docs/api.md](../../../../docs/api.md) sections "Ads" and "Admin panel → Events, payments, plans, moderation, ads".

| Method | Path | Group | Permission |
|---|---|---|---|
| GET | `/ads?placement&sportId&state` | verified | |
| POST | `/ads/:id/impression` | verified | |
| POST | `/ads/:id/click` | verified | |
| GET | `/admin/ads?status` | admin | `ads.write` |
| POST | `/admin/ads` | admin | `ads.write` |
| PUT | `/admin/ads/:id` | admin | `ads.write` |
| GET | `/admin/ads/:id/stats?from&to` | admin | `ads.write` |

## Tables

- Owned: `ad_campaigns`, `ad_daily_stats`.
- Read: `sports` (validate `sportId`), `media_uploads` (through `platform/media`: claim and sign ad images).

## Events

None published or consumed.

## Jobs

- `ads.end_campaigns` (hourly): up to 500 `active` / `paused` campaigns whose `ends_at` has
  passed → `ended`. Idempotent (already ended rows are not matched). Serving also filters on
  `status = 'active'` and `starts_at <= now < ends_at`, so a campaign stops showing at
  `ends_at` even before the job runs; the job keeps the admin status true. `draft` campaigns
  are left alone.

## Config keys

| Key | Default | Use |
|---|---|---|
| `ads.enabled` | `false` | off → `GET /ads` returns `[]`, impression/click return 404 |
| `ads.show_to_minors` | `false` | off → minors get `[]` / 404 |
| `ads.feed_every_n_posts` | `8` | read by the app only (public config) |

## Rules

- Targeting is contextual only: placement, sport, state. No sport / state in the request means
  only untargeted campaigns (NULL sport / NULL state) match.
- Order: `priority DESC, id DESC`; at most `ads.max_per_request` ads per request (default 3, bounded 1..10).
- Every served ad carries `label: "Sponsored"`.
- Impression / click count only for an ad that is servable right now and to this user (same
  enabled / minor rules), else 404. Counters are per `(ad_id, IST date)`; no user id is stored.
- `targetUrl` must be an absolute `https://` URL without credentials, at most 500 chars.
- Ad images are admin uploads owned by user id 0 with purpose `post_image`; they are claimed via
  `d.Media.Claim` inside the write transaction. Image URLs are signed per response.
- `PUT` is partial: a missing key is unchanged, `null` clears a nullable field.
- Every admin write runs in one transaction with `audit.Admin` (ids / status only in details).
- Stats: `from` / `to` are `YYYY-MM-DD` (IST days), default the last 30 days, max 366 days,
  zero-filled for days with no traffic.

## Tests

`ads_test.go`: disabled → `[]`, contextual query args, untargeted-only without context,
minor rule both ways, bad placement 422, impression / click upserts on the IST date,
non-servable / disabled / minor → 404, admin permission 403, create validation (URL, placement,
dates, status, body length), create with image claim + audit, foreign image 422, update 404 and
partial update, list paging and status filter, stats validation / zero-fill / default range / 404,
`ads.end_campaigns` job (query, rerun no-op, DB error returned).

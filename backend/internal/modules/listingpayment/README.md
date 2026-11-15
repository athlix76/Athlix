# listingpayment

The listing fee an organiser pays to put an event up for review / publication. This module
quotes fees from `listing_fee_rules`, creates orders (Razorpay, mock outside production, or
"none" for a zero total), records payments from Checkout and webhooks, moves the event to
`pending_review` / `published`, and refunds when an event is cancelled, rejected or refunded by an admin.

## Routes

See [docs/api.md](../../../../docs/api.md) sections "Listing fee" and "Admin panel → Events, payments, plans, moderation, ads".

| Method | Path | Group | Permission |
|---|---|---|---|
| GET | `/listing/quote?sportId&eventType&entryFeePaise&maxSlots` | verified | |
| POST | `/events/:id/listing/quote` | verified (owner) | |
| POST | `/events/:id/listing/order` | verified (owner) | |
| GET | `/events/:id/listing/order` | verified (owner; latest order + status, 404 when none or not the owner) | |
| POST | `/events/:id/listing/confirm` | verified (owner) | |
| POST | `/listing/mock/pay` | verified (owner; mock provider outside prod only) | |
| POST | `/webhooks/razorpay` | webhooks (signature checked first) | |
| GET | `/admin/listing-orders?status&eventId&from&to&cursor&limit` | admin | `payments.read` |
| POST | `/admin/listing-orders/:id/refund` | admin | `payments.refund` |
| GET | `/admin/fee-rules` | admin | `config.read` |
| POST | `/admin/fee-rules` | admin | `fees.write` |
| PUT | `/admin/fee-rules/:id` | admin | `fees.write` |
| POST | `/admin/fee-rules/preview` | admin | `config.read` |

## Tables

- Owned: `listing_orders`, `listing_fee_rules`, `payment_webhook_events` (Razorpay rows).
- Written by exception: `events.status` / `events.published_at` (pending_payment, pending_review,
  published, back to draft on expiry), as allowed by the brief.
- Read: `events` (owner-scoped, `FOR UPDATE` while paying), `policy_documents` (active
  `listing_refund` doc id), users via `d.Users.Summaries` (admin list).
- Audit: `admin_audit_log` (refund, fee rule create/update), `config_audit` (fee rules).

## Events

- Published (after commit): `listing.paid`, `listing.refunded` (amount = this refund),
  `event.submitted` (paid, approval required) or `event.published` (paid, no approval).
- Consumed: `event.cancelled`, `event.rejected`, `event.updated`.

## Jobs

`listing.expire_orders`: orders in `created` / `failed` past `expires_at` become `expired` and
their event returns from `pending_payment` to `draft` when it has no other live open order.
Batches of 500, re-runnable.

## Config keys

| Key | Default | Use |
|---|---|---|
| `listing.payment_provider` | `mock` | `razorpay` or `mock`; mock in prod → 502 |
| `listing.order_ttl_minutes` | `30` | order `expires_at` |
| `listing.refund.before_first_registration_percent` | `100` | cancel with 0 registrations |
| `listing.refund.after_registrations_percent` | `0` | cancel with registrations |
| `listing.refund.on_admin_rejection_percent` | `100` | admin rejection |
| `event.require_admin_approval` | `true` | paid → pending_review (else published) |
| `event.min_hours_before_start` | `24` | submit check on order |
| `event.allow_free_events` | `true` | free events (quote and order) |
| `event.max_slots_limit` | `1024` | maxSlots bound |

Secret: `app.SecretRazorpay` through `d.Secrets` + `d.Cfg.SecretName`.

## Rules

- Fee: `base = entryFee == 0 ? freeEventFee : clamp(round(entryFee × maxSlots × percentBp / 10000), min, max)`,
  `gst = round(base × gstBp / 10000)`, half-up to the paisa, NULL max = no cap. Money is int64
  paise, percentages basis points.
- Rule choice among active rules in their window that match sport/type or are wildcards:
  most specific (sport+type > sport > type > default), then priority DESC, then id DESC. No rule → 409.
- An order snapshots the quote. Re-ordering reuses an open `created` order with the same quote
  that has not expired; other open orders are expired. A zero total is paid at once (provider `none`).
- Payments (confirm, webhook, mock pay) lock the event, then the order. A repeat is a no-op.
  A payment on an order that is no longer payable (expired, or the event left `pending_payment`)
  is recorded and refunded in full; confirm / mock pay then return 409.
- Confirm verifies the Checkout signature with the key secret (mismatch → 422). The webhook
  verifies `X-Razorpay-Signature` before any SQL (401), dedupes on the event id
  (`X-Razorpay-Event-Id`, else sha256 of the body) and stores ids and type only, never the payload.
  A captured amount different from the order total is logged and not settled.
- Cancel / reject refunds apply only to orders still `paid`, so re-delivery refunds nothing twice;
  a `removed` event is not refunded. Razorpay refunds get `refunded_at` on `refund.processed`.
- Admin refunds: amount ≤ remaining total, reason 1–255 chars, audited in the same transaction.
- The last active default fee rule cannot be deactivated or narrowed (409).
- Mock pay is 404 unless the provider is `mock` and the env is not prod.
- Logs carry ids only; Razorpay errors are logged and returned as a generic 502.

## Tests

`listingpayment_test.go`: fee maths (percent, min/max clamp, free, nil max, half-up), rule
choice, percentOf, date parsing; quote happy path, refund settings, specific rule, no rule 409,
422s and config limits, event quote owner 404; create order (mock, TTL setting, reuse, expire
changed quote, zero total with/without approval, Razorpay, provider failure, mock in prod 502,
wrong status 409, non-owner 404, submit check 422); confirm (happy, bad signature, validation,
idempotent, non-owner 404, mock order 409, late payment refund); mock pay (disabled, 422, happy,
non-owner 404); webhook (bad signature without SQL, captured + duplicate, already paid, unknown
order ignored with body-hash id, amount mismatch, failed, order.paid, refund.processed + duplicate);
handlers (cancel before / after registrations with setting, idempotent re-delivery, removed,
missing event, rejected via Razorpay, rejection setting, resubmitted, updated); expiry job and
re-run; admin 403 on every route, list with organiser and cursor, refund happy / 422 / 409 / 404,
fee rule list, create + audit, create 422, update last-default 409 and allowed, partial update,
update 404 / 422, preview.

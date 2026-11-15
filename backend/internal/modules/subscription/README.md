# subscription

Premium plans, in-app purchases (Google Play Billing and Apple IAP) and manual grants. The
server checks every purchase with the store and never trusts the client or a store
notification body. Benefits are read by other modules through `platform/entitlements`. This
module owns only the plan catalogue and the subscription rows.

## Routes

Sections in [docs/api.md](../../../../docs/api.md): "Premium" and admin "Events, payments,
plans, moderation, ads".

| Method | Path | Group | Permission |
|---|---|---|---|
| GET | `/v1/premium/plans` | verified | – |
| GET | `/v1/me/subscription` | verified | – |
| POST | `/v1/premium/purchases/verify` | verified | – |
| POST | `/v1/premium/purchases/restore` | verified | – (`{purchases: [...]}`, at most `premium.restore_max_purchases`, each re-verified with the store) |
| POST | `/v1/premium/mock/activate` | verified (ENV ≠ prod; 404 in prod) | – |
| POST | `/v1/webhooks/google-play` | webhook (Pub/Sub OIDC JWT) | – |
| POST | `/v1/webhooks/app-store` | webhook (signed JWS) | – |
| GET | `/v1/admin/plans` | admin | `config.read` |
| POST | `/v1/admin/plans` | admin | `plans.write` |
| PUT | `/v1/admin/plans/:id` | admin | `plans.write` |
| PUT | `/v1/admin/plans/:id/benefits` | admin | `plans.write` |
| GET | `/v1/admin/subscriptions` | admin | `payments.read` |
| POST | `/v1/admin/subscriptions/grant` | admin | `plans.write` |

Status codes this module chooses:

| Case | Response |
|---|---|
| The store purchase is missing, is for another product, or carries another account token | 404 `NOT_FOUND` (Purchase) |
| The purchase is already recorded for another user | 404 `NOT_FOUND` (Purchase) |
| Play payment still pending | 409 `INVALID_STATE` |
| Store secret missing or incomplete | 409 `INVALID_STATE` on verify; 401 on the webhook |
| Store API down or unexpected reply | 502 `UPSTREAM_ERROR` |
| Product id not mapped to any plan | 422, `fields.productId` |
| Mock activate while `premium.enabled` is false | 409 `INVALID_STATE` |
| Webhook signature or token invalid | 401 `UNAUTHENTICATED` |
| Webhook processing failed (for example the store API is down) | 5xx, so the store redelivers |

## Tables

- **Owned:**
  - `premium_plans`
  - `premium_plan_benefits`
  - `subscriptions`
  - `payment_webhook_events`, rows with provider `google_play` or `app_store` only. `listingpayment` owns the Razorpay rows.
- **Written through platform/audit:** `admin_audit_log` and `config_audit`.
- **Read only:** `users` and `profiles` through `d.Users` (admin list, grant target), and the entitlements benefit query.

`store_transaction_id`:

- **Play:** hex SHA-256 of the purchase token. The token stays the same across renewals.
  `purchase_token_hash` holds the same 32 bytes.
- **App Store:** `originalTransactionId`. `purchase_token_hash` is NULL.
- **Manual:** NULL.

The raw purchase token is never stored.

## Events

- **Published, after commit:** `subscription.activated` and `subscription.expired`, with
  `SubscriptionPayload{subscriptionId, userId, planId}`.
  - `activated` fires when a row becomes `active`/`grace` from no row or from a non-live status.
  - `expired` fires when a live row becomes `expired`, `cancelled` or `refunded`. This includes
    the older Play purchase replaced through `linkedPurchaseToken`, and the job.
- **Consumed:** none.

## Jobs

`subscription.expire` runs hourly:

- In one transaction it locks up to 500 `active`/`grace` rows with `expires_at <= now`
  (`FOR UPDATE`) and sets them to `expired`. The UPDATE repeats the status and expiry
  conditions.
- After commit it emits `subscription.expired` for each row. Re-running it is safe.

Grace periods come from the store: Play `IN_GRACE_PERIOD`, and Apple status 4 with
`gracePeriodExpiresDate` used as the expiry.

## Config keys

- `premium.enabled` (bool, default `false`):
  - When false, `GET /premium/plans` returns `[]` and mock activate is refused.
  - Store purchases are still verified and honoured, because the user has already paid.
- **Secrets** (docs/security.md), read with `d.Secrets` and `d.Cfg.SecretName`:
  - `google-play`: `packageName`, `clientEmail`, `privateKey`, `pubsubAudience`, `pubsubServiceAccountEmail`
  - `app-store`: `bundleId`, `issuerId`, `keyId`, `privateKey`, `environment`

  Values are cached in memory only: the Play OAuth token and the Google JWKS (1 h TTL, at most
  one refetch a minute).
- Plans, prices, store product ids and benefits are DB config in `premium_plans` and
  `premium_plan_benefits`. Money is in paise.

## Rules

- **Account binding:** the app passes `Me.storeAccountToken` as the Play
  `obfuscatedAccountId` / Apple `appAccountToken`.
  - Verify returns 404 unless the store purchase carries the caller's token.
  - The token is `Box.Hash("store_account", userID)[:16]` with the UUID v4 and variant bits
    set. `storeAccountToken` must stay equal to `user.StoreAccountToken`. The test pins the
    format.
- **Row ownership:** a row is found by `(store, store_transaction_id)` and locked with
  `FOR UPDATE`. If it belongs to another user, the result is 404 and nothing is written.
- **Webhooks:**
  - The signature is checked before anything else.
    - **Play:** the `Authorization` header is verified (RS256 Google JWKS, `aud`, `iss`,
      `email`, `email_verified`, `exp`) before the body is read.
    - **Apple:** the JWS x5c chain must be leaf, intermediate, root. The root is pinned to the
      Apple Root CA G3 SHA-256 fingerprint, both Apple marker extensions are required, and the
      leaf must be valid at `now`. The signature is ES256.
  - The payload is never stored. Only `provider_event_id` (Pub/Sub `messageId` or
    `notificationUUID`), the type, our subscription id and the status are kept.
  - Dedupe: a `processed` or `ignored` event returns 200 without work. A `received` or
    `failed` event is processed again.
  - State always comes from re-fetching the store API, never from the message.
  - Notifications for tokens we have never verified are `ignored`. Rows are bound to users only
    through `POST /premium/purchases/verify`.
- **Status mapping:**
  - **Play:** `ACTIVE` and `CANCELED` stay `active` until expiry. `IN_GRACE_PERIOD` is `grace`.
    `REVOKED` (RTDN 12) is `refunded`. Anything else is `expired`.
  - **Apple:** revoked or status 5 is `refunded`, 1 is `active`, 4 is `grace`. Anything else is
    `expired`.
- **Acknowledgement:** live Play purchases are acknowledged after commit. A failure is only
  logged; the app and the next RTDN retry.
- **Admin writes:** every admin write runs `audit.Admin` in the same transaction. Plan and
  benefit writes also run `audit.Config` with old and new values. Benefit keys come from the
  closed list in `platform/entitlements`: booleans must be `true` and ints positive. A store
  product id may belong to only one plan (409).
- **Logs:** ids only. Purchase tokens, JWTs, secrets and store responses are never logged or
  wrapped in errors.

## Tests

`subscription_test.go` (sqlmock with fake stores):

- Plans: off gives `[]`, on gives the list, and an unknown audience gives 422.
  `/me/subscription` with and without a row.
- Verify on Play and Apple:
  - happy paths; only the hash is stored; the activation event and the ack
  - account token mismatch, product mismatch, and another user's row (each 404)
  - 502, 404, pending 409, store not configured 409, and validation
  - re-verify updates the row with no second event
  - an upgrade cancels the linked row
  - status mapping tables
- Mock activate: happy path, premium off (409), inactive plan (404), validation, and prod (404).
- Webhooks:
  - Play renewal, revoke to refunded, ignored cases (other package, test notification, unknown
    token), and a bad body (400)
  - bad auth gives 401 without reading the body
  - a duplicate gives 200 with no work; a failed event is retried
  - Apple expiry, duplicate, not ours, and bad signature
- Admin:
  - 403 for each route with the wrong permission
  - list with inactive plans
  - create, including validation and duplicate code (409)
  - update: 404, product clash (409), partial update with null fields
  - benefit validation and replace with both audits
  - subscriptions page with cursor and user summary
  - grant: 422, unknown user 404, happy path
- Job: expires a batch with events; a re-run is a no-op; a failure rolls back.

`stores_test.go` (real crypto and httptest, no network):

- The JWS verifier with a generated root, intermediate and leaf chain: accepted, and rejected
  for the real Apple pin, a forged or tampered token, HS256, a short chain, a missing marker,
  an expired leaf, and another root.
- The App Store client: statuses, the API token claims, bundle filtering, an unpinned reply,
  404, no secret, and the notification `Ours` checks.
- The Pub/Sub verifier: valid token, JWKS caching, every claim failure, a forged token, an
  unknown kid refetched at most once a minute, and no secret.
- The Play client: OAuth exchange cached, v2 mapping (latest line item), ack, 410 to not
  found, 500 to upstream with no URL in the error, and no secret.

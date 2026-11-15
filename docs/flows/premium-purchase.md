# Premium purchase

Premium plans are auto-renewing subscriptions sold through **Google Play Billing** and
**Apple in-app purchase**. Plans, prices, store product ids and benefits are rows in
`premium_plans` / `premium_plan_benefits`. The server verifies every purchase with the store
and never trusts the client. Plans are shown only when `premium.enabled` is true.

Purchases are bound to the Athlix account by `Me.storeAccountToken` (a UUID derived from a
keyed hash of the user id), passed to the store as Play `obfuscatedAccountId` / Apple
`appAccountToken`. A purchase carrying another account's token is refused (404), so a
receipt cannot be claimed by someone else. Purchase tokens are stored only as a SHA-256 hash.

## Buy and verify

```mermaid
sequenceDiagram
    autonumber
    actor U as User
    participant App as Athlix app
    participant API as Athlix API
    participant ST as Play or App Store
    participant W as Worker

    App->>API: GET /premium/plans?audience=coach
    API-->>App: Plan list, empty when premium.enabled is false
    App->>API: GET /me
    API-->>App: Me with storeAccountToken
    U->>App: Choose Coach Pro
    App->>ST: Start purchase of storeProductIdAndroid or storeProductIdIos with storeAccountToken
    U->>ST: Confirm and pay
    ST-->>App: purchaseToken on Android or transactionId on iOS
    App->>API: POST /premium/purchases/verify {store, productId, purchaseToken, transactionId}
    API->>ST: Fetch the purchase from the Play Developer API or App Store Server API
    alt Store account token differs from this user
        API-->>App: 404 NOT_FOUND
    else Store API fails
        API-->>App: 502 UPSTREAM_ERROR
    else Product not in premium_plans or purchase not valid at the store
        API-->>App: Error, nothing activated
    else Valid
        Note over API: upsert subscription by purchase_token_hash, status active, expiresAt from the store
        API-->>App: Subscription
        Note over API: emits subscription.activated
        W->>U: Notification plan active
    end
    App->>API: GET /me/subscription
    API-->>App: {active, benefits}
```

- Verify is idempotent: the same purchase verified twice returns the same subscription.
- Benefits come from the closed list in [config-system.md](../config-system.md#premium-benefits-closed-list);
  if several active plans share a key, the larger value wins.
- `POST /premium/mock/activate {planId}` replaces the store outside prod.
- Admins can grant a plan manually: `POST /admin/subscriptions/grant {userId, planId, days, reason}`
  (store `manual`, permission `plans.write`).

## Renewals, cancellations, refunds

```mermaid
sequenceDiagram
    autonumber
    participant GP as Google Play via Pub/Sub
    participant AS as App Store
    participant API as Athlix API
    participant ST as Store APIs
    participant W as Worker

    alt Android RTDN
        GP->>API: POST /webhooks/google-play with Authorization Bearer OIDC JWT
        Note over API: verify RS256 against Google certs, aud equals pubsubAudience, email equals pubsubServiceAccountEmail
        API->>ST: Re-fetch the subscription from the Play Developer API
    else iOS Server Notification v2
        AS->>API: POST /webhooks/app-store {signedPayload}
        Note over API: verify the x5c chain to Apple Root CA G3 and the ES256 signature
        API->>ST: Re-fetch the transaction from the App Store Server API
    end
    Note over API: dedupe on payment_webhook_events, never trust or store the message body
    Note over API: update status and expiresAt
    API-->>GP: 200, also for duplicates and ignored types
    W->>W: Job subscription.expire hourly, past expiresAt becomes expired
    Note over W: emits subscription.expired, benefits stop
```

## Subscription status

```mermaid
stateDiagram-v2
    [*] --> active: verified purchase or manual grant
    active --> active: renewal, expiresAt extended
    active --> grace: renewal payment failed, store grace period
    grace --> active: payment recovered
    grace --> expired: grace ended
    active --> cancelled: auto renew off, benefits until expiresAt
    cancelled --> expired: expiresAt passed
    cancelled --> active: resubscribed
    active --> refunded: store refund or revoke
    expired --> [*]
    refunded --> [*]
```

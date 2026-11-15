# features/premium

Premium plans and in-app purchase (Google Play Billing / Apple IAP).

## Screens

| Screen | Route | Notes |
|---|---|---|
| `PremiumScreen` | `/premium` | current subscription, plans grouped by audience (mine first), subscribe, restore, test activation outside prod |

## Endpoints

- `GET /premium/plans`, `GET /me/subscription`
- `POST /premium/purchases/verify` `{store, productId, purchaseToken | transactionId}`
- `POST /premium/mock/activate` `{planId}` (only when `Env.showMockFlows`)

## Purchase flow

1. Store products are loaded for the plan's `storeProductIdAndroid` / `storeProductIdIos`.
2. `buyNonConsumable`, with `Me.storeAccountToken` passed as `applicationUserName`
   (Play obfuscatedAccountId / Apple appAccountToken).
3. On `purchased` / `restored`, call `POST /premium/purchases/verify`:
   - Android sends `serverVerificationData` as `purchaseToken`.
   - iOS sends `purchaseID` as `transactionId`.
4. Call `completePurchase` only after the server accepts. A failed verify is left
   unfinished, so the store redelivers it.

## Rules

- Prices come from the store listing or `pricePaise`; never hard-code a price or benefit.
- The purchase token goes to our server only; it is never logged or stored on the device.

## Changing it safely

New store state: handle it in `PremiumPurchaseController._onPurchases` and always either
verify or `completePurchase`, so no purchase is left hanging.

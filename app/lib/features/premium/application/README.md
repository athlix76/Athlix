# premium/application

- `premium_controller.dart`:
  - `premiumPlansProvider`, `mySubscriptionProvider`
  - `storeProductsProvider`, which is empty when the store is unavailable
  - `storeProductIdFor`
  - `PremiumPurchaseController`: buy, restore, verify, complete, mock

All store calls are wrapped: emulators and tests without a store just show plan prices.

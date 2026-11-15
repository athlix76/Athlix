# ads/data

- `ad_models.dart`: `Ad` (fields match docs/api.md, Ads) and `AdQuery` (placement, sportId, state).
- `ad_api.dart`: `AdApi` (`list`, `impression`, `click`) and `adApiProvider`.

Changing it safely: keep field names identical to the contract. Tracking calls are fire and
forget; callers must not show their errors.

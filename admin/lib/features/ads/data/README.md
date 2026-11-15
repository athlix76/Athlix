# features/ads/data

API calls and models for this feature. One class per endpoint group, plain Dart, no widgets.

| File | What it is |
|---|---|
| `ads_api.dart` | Endpoint calls and response models. |

## Changing it safely

Keep paths and field names exactly as docs/api.md. Parse with `core/api/json.dart` helpers. Never log request or response bodies.
See `../README.md` for the endpoints and permissions.

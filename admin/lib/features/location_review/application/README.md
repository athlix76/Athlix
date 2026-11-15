# features/location_review/application

Riverpod providers (and pure helpers) that connect the data layer to the screens.

| File | What it is |
|---|---|
| `location_review_providers.dart` | Riverpod providers for this feature. |
| `proof_link.dart` | When a signed proof URL stops working, measured from when the detail was fetched (so the admin's clock skew does not matter). |

## Changing it safely

Keep pure logic here so it can be unit-tested without widgets. Use `autoDispose` for anything that holds user data.
See `../README.md` for the endpoints and permissions.

# features/plans/application

Riverpod providers (and pure helpers) that connect the data layer to the screens.

| File | What it is |
|---|---|
| `plans_providers.dart` | Riverpod providers for this feature. |

## Changing it safely

Keep pure logic here so it can be unit-tested without widgets. Use `autoDispose` for anything that holds user data.
See `../README.md` for the endpoints and permissions.

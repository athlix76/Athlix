# features/config/application

Riverpod providers (and pure helpers) that connect the data layer to the screens.

| File | What it is |
|---|---|
| `config_providers.dart` | Riverpod providers for this feature. |
| `config_value_codec.dart` | Parses and validates editor text per config value type; text form for diffs; deep value compare. |

## Changing it safely

Keep pure logic here so it can be unit-tested without widgets. Use `autoDispose` for anything that holds user data.
See `../README.md` for the endpoints and permissions.

# features/fee_rules/application

Riverpod providers (and pure helpers) that connect the data layer to the screens.

| File | What it is |
|---|---|
| `fee_formula.dart` | Display-only mirror of the backend listing fee formula, for the live preview of unsaved rules. |
| `fee_rules_providers.dart` | Riverpod providers for this feature. |

## Changing it safely

Keep pure logic here so it can be unit-tested without widgets. Use `autoDispose` for anything that holds user data.
See `../README.md` for the endpoints and permissions.

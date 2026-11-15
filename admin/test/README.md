# test

Unit tests for the parts of the panel where a mistake is a security or money bug. They are
plain Dart logic tests (no widgets, no network), so they run fast with `flutter test`.

| File | What it covers |
|---|---|
| `core/permissions_test.dart` | `hasPermission` / `*`, sidebar gating, landing page, route ownership |
| `core/api_error_test.dart` | API error envelope and status mapping, Dio failures, client-side validation errors |
| `features/config_value_codec_test.dart` | Config value parsing and validation per type, text form, deep compare |
| `features/fee_formula_test.dart` | Listing fee preview mirror (paise, basis points, clamps, free events) |
| `features/proof_link_test.dart` | Expiry of signed proof URLs for the location proof viewer |

## Running

```sh
flutter test
```

## Changing it safely

- Numbers in tests are fixtures, not business defaults. Never copy them into `lib/`.
- No real PII, tokens or URLs from any environment in fixtures.
- When you change a tested file, change or add the test in the same change.

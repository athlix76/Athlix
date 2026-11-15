# app/test

Plain Dart unit tests (no widgets, no device). Run with `flutter test`.

| File | Covers |
|---|---|
| `envelope_test.dart` | `parseEnvelope`, `apiExceptionFromBody`, `Paged.fromEnvelope` |
| `error_mapping_test.dart` | `ApiErrorCode.fromWire` / `fromStatus`, `errorMessage`, `mapDioException`, `NOT_ELIGIBLE` fields |
| `token_refresher_test.dart` | single-flight refresh, rejected vs failed refresh, redacted `TokenPair` |
| `money_test.dart` | integer-paise formatting and parsing (no floating point) |
| `redirect_test.dart` | `computeRedirect`: gates, signed out, policies, each onboarding step, deep link |
| `models_test.dart` | `Me`, `ListingQuote`, `Message` round trips; `EventSummary` eligibility; `Registration` flag |
| `eligibility_view_test.dart` | `EligibilityView`, reason/action mapping, `NotEligibleInfo`, `SubmitProblem`, `EligibilityInput.toJson`, distance filter query |
| `notification_routing_test.dart` | push / in-app notification data to route |
| `version_test.dart` | force-update version compare |

## Changing it safely

- Keep tests pure: build models from JSON maps shaped like docs/api.md, never from real user data.
- Fixtures use made-up names and masked numbers only. No tokens, no real mobiles, no PII.
- When a wire field or error code changes in docs/api.md, update the fixture here in the same change.

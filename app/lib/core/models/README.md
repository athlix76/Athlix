# core/models

Models shared by several features: `Me` and `PendingPolicy` (`me.dart`), `UserSummary`, `Sport` / `SportRef`,
policy documents. Field names match docs/api.md.

## Changing it safely

Add fields to both `fromJson` and `toJson`, keep them nullable or defaulted so older servers still parse, and
extend `test/models_test.dart`. `Me.mobileMasked` is already masked by the server; do not add raw PII fields.

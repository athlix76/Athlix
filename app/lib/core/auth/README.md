# core/auth

Session tokens.

- `token_pair.dart`: `TokenPair` (access + refresh, expiries). `toString()` is redacted.
- `token_store.dart`: `TokenStore` interface and `SecureTokenStore` (flutter_secure_storage only).
- `token_refresher.dart`: single-flight `POST /auth/refresh`. Refresh tokens rotate and reuse revokes the
  session, so concurrent 401s share one refresh call.
- `auth_interceptor.dart`: adds `Authorization`, retries once after refresh, signals session expiry.

## Changing it safely

Tokens are stored only in secure storage, never in preferences, files, logs, analytics or crash reports.
Keep `test/token_refresher_test.dart` green; never allow two refreshes in flight.

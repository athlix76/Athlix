# core/api

HTTP access to the Athlix API.

- `api_client.dart`: `ApiClient` (`get`, `page`, `post`, `put`, `patch`, `delete`, `send`) on dio, and
  `mapDioException` which turns any dio failure into an `ApiException`.
- `envelope.dart`: `parseEnvelope` (`{data, meta?}` / `{error}`), `PageMeta`, `Paged<T>`.
- `api_exception.dart`: `ApiErrorCode` (wire codes from docs/api.md plus client-side `network` / `unknown`)
  and `ApiException` (`code`, safe `message`, string `fields`, `statusCode`, `requestId`).
- `api_signals.dart`: a broadcast bus for session expired / onboarding required / suspended / maintenance.

## Changing it safely

- Switch on `ApiErrorCode`, never on messages. A new wire code goes into the enum with its exact string.
- Never put request or response bodies, headers or tokens into exceptions, logs or `toString()`.
- Keep `test/envelope_test.dart` and `test/error_mapping_test.dart` passing.

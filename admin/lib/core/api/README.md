# lib/core/api

HTTP layer: the single `ApiClient` (auth header, refresh, envelope unwrap), `ApiError` and its code mapping, JSON readers, cursor pages.

| File | What it is |
|---|---|
| `api_client.dart` | The only HTTP client of the panel. |
| `api_error.dart` | Error codes from docs/api.md (Conventions → Envelope), plus two client-side codes for failures that never reached the server. |
| `json.dart` | Small, forgiving readers for decoded JSON. |
| `page.dart` | One page of a cursor-paginated list (`meta.nextCursor`, absent on the last page). |

## Changing it safely

- `ApiClient` must never log requests, bodies or tokens. Do not add a logging interceptor.
- Screens switch on `ApiError.code`, never on the message. New backend codes go in `ErrorCodes` with a title and hint.
- Covered by test/core/api_error_test.dart.

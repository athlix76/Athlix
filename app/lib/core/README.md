# lib/core

Shared building blocks used by every feature.

| Folder | What |
|---|---|
| `api/` | dio client, envelope parser, error codes, cross-cutting API signals |
| `auth/` | token pair, secure token store, single-flight refresher, auth interceptor |
| `config/` | `GET /bootstrap`, `AppConfig` getters for config keys, force-update check |
| `format/` | date and text display helpers |
| `json/` | tolerant JSON readers (`asInt`, `asString`, ...) and RFC 3339 formatting |
| `location/` | one-shot coarse device location for the distance filter |
| `media/` | pick images / documents and the upload ticket flow |
| `models/` | models used by many features (`Me`, `UserSummary`, `Sport`, policies) |
| `money/` | integer-paise formatting and parsing |
| `router/` | route constants, go_router setup, the redirect rule |
| `session/` | signed-in state and `Me` |
| `theme/` | Material 3 theme |
| `widgets/` | reusable widgets (errors, lists, pickers, avatars) |
| `ws/` | chat WebSocket with backoff |

`env.dart` holds the two `--dart-define` values (`API_BASE_URL`, `ENV`). Nothing secret.

## Changing it safely

Changes here affect every screen: keep APIs small and add a unit test in `app/test/` for logic.

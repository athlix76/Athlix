# lib/core

Code shared by every feature. Nothing in here knows about a specific screen.

| Folder / file | What it is |
|---|---|
| `api/` | `ApiClient`, `ApiError`, JSON helpers, cursor pages |
| `auth/` | Permissions, `AdminMe`, in-memory token store, idle watcher |
| `config/` | `--dart-define` settings |
| `format/` | Money (paise), basis points, dates, line diff |
| `media/` | `pickFile` (browser file picker on web, `null` elsewhere) for uploads |
| `models/` | Models used by more than one feature |
| `router/` | Routes and the sidebar list |
| `widgets/` | Shared widgets (shell, gates, dialogs, lists, form fields) |
| `providers.dart` | The token store and API client providers |

## Changing it safely

- A change here affects every screen. Run `flutter analyze` and `flutter test` before merging.
- Feature code may import `core/`; `core/` must not import `features/`, except
  `router/app_router.dart` (it lists the screens) and `widgets/admin_shell.dart` /
  `widgets/permission_gate.dart` (they read the session from `features/auth`).

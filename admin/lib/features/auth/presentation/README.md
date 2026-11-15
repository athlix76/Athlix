# features/auth/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `login_screen.dart` | Email + password + TOTP code, in one request (`POST /admin/auth/login`). |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

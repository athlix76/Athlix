# features/admins/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `admins_screen.dart` | Admin users and roles (admins.manage). |
| `one_time_credentials_dialog.dart` | Shows a temporary password and the TOTP enrolment QR once. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

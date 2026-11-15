# features/audit/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `audit_log_screen.dart` | Read-only list of every admin write and every sensitive read (proof views, message reads, verified-name reveals). |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

# features/dashboard/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `dashboard_screen.dart` | Headline counts from `GET /admin/dashboard` with links to the queues. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

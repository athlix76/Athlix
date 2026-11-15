# features/subscriptions/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `subscriptions_screen.dart` | Subscriptions list with filters, and manual grant. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

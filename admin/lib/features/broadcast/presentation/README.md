# features/broadcast/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `broadcast_screen.dart` | Sends one push + in-app notification to an audience. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

# features/communities/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `communities_screen.dart` | Communities list with search and status filter; remove with a reason. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

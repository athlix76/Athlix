# features/ads/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `ads_screen.dart` | Ad campaigns: list, create, edit, pause/resume and daily stats. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

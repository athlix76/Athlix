# features/sports/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `sports_screen.dart` | Sports, their positions and stat templates. |
| `stat_template_editor.dart` | Create or edit a stat template. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

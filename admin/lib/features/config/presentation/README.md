# features/config/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `config_screen.dart` | Runtime config (`app_config`), grouped by category. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

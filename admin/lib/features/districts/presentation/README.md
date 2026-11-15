# features/districts/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `aliases_field.dart` | Edits a list of alias names as chips. |
| `districts_screen.dart` | Districts used by location identities and event eligibility. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

# features/verification_requirements/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `verification_requirements_screen.dart` | Which checks each role needs, and what each check unlocks (gates). |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

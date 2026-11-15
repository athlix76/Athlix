# features/organisers/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `organiser_detail_screen.dart` | One application: steps, documents, approve / reject. |
| `organisers_screen.dart` | Organiser applications queue. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

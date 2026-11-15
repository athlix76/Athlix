# features/events/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `event_detail_screen.dart` | One event: details, category and eligibility summary, approve / reject / remove. |
| `events_screen.dart` | Event review queue with filters. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

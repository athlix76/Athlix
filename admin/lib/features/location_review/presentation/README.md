# features/location_review/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `location_detail_screen.dart` | One identity: summary, reviewer checklist, proof viewer, history, approve / reject / revoke. |
| `location_review_screen.dart` | Queue of native / residence identities. |
| `proof_viewer.dart` | Shows the proof behind a short-lived signed URL. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

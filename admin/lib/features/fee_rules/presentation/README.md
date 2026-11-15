# features/fee_rules/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `fee_preview_card.dart` | Live fee calculator against the saved rules (`POST /admin/fee-rules/preview`). |
| `fee_rules_screen.dart` | Listing fee rules. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

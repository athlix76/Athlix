# features/payments/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `listing_orders_screen.dart` | Listing fee orders (the only money Athlix collects for events) and refunds. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

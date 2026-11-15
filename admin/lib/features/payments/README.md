# features/payments

Listing-fee payments and refunds.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `ListingOrdersScreen` | `/listing-orders` | orders, filters, refund |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/listing-orders` | payments.read |
| POST | `/admin/listing-orders/:id/refund` | payments.refund |

## Permissions

View `payments.read`; refund needs `payments.refund`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Amounts are integer paise. The refund amount is entered in rupees and converted with `parseRupeesToPaise`.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

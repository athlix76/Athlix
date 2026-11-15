# features/subscriptions

Subscriptions list and manual grants.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `SubscriptionsScreen` | `/subscriptions` | list, filters, grant |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/subscriptions` | payments.read |
| POST | `/admin/subscriptions/grant` | plans.write |
| GET | `/admin/plans` | config.read (plan picker for grants) |

## Permissions

View `payments.read`; grant needs `plans.write` (and `config.read` for the plan list).

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

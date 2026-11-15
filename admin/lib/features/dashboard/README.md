# features/dashboard

Headline counts for operations.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `DashboardScreen` | `/dashboard` | counts with links to the queues |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/dashboard` | dashboard.read |

## Permissions

`dashboard.read`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Money values are paise; show them with `formatPaise`.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

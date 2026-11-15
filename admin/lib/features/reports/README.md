# features/reports

User reports queue and resolution.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `ReportsScreen` | `/reports` | queue (default open), resolve, links to the target |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/reports` | reports.read |
| POST | `/admin/reports/:id/resolve` | reports.resolve |

## Permissions

View `reports.read`; resolve needs `reports.resolve`; "Read messages" needs `moderation.read_messages`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

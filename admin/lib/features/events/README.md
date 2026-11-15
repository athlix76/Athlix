# features/events

Event review queue and event detail, including category and location eligibility.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `EventsScreen` | `/events` | queue and filters |
| `EventDetailScreen` | `/events/:id` | details, category and eligibility, approve / reject / remove, feature / unfeature (published only), registrations dialog (status filter, flagged only) |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/events` | events.review |
| GET | `/admin/events/:id` | events.review |
| POST | `/admin/events/:id/approve` | events.review |
| POST | `/admin/events/:id/reject` | events.review |
| POST | `/admin/events/:id/remove` | events.review |
| POST | `/admin/events/:id/feature` | events.review |
| POST | `/admin/events/:id/unfeature` | events.review |
| GET | `/admin/events/:id/registrations` | events.review |

## Permissions

`events.review`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Feature and unfeature are separate fixed paths (no path built from a flag), so the API
  cross-check can match them.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

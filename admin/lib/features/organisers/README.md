# features/organisers

Organiser applications queue and review.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `OrganisersScreen` | `/organisers` | queue |
| `OrganiserDetailScreen` | `/organisers/:userId` | steps, documents, approve / reject |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/organisers` | organisers.review |
| GET | `/admin/organisers/:userId` | organisers.review |
| POST | `/admin/organisers/:userId/approve` | organisers.review |
| POST | `/admin/organisers/:userId/reject` | organisers.review |

## Permissions

`organisers.review`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Document links are short-lived signed URLs from the backend; open them, never store them.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

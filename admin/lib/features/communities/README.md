# features/communities

Communities list and removal.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `CommunitiesScreen` | `/communities` | search, status filter, remove with a reason |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/communities` | communities.moderate |
| POST | `/admin/communities/:id/remove` | communities.moderate (204) |

## Permissions

`communities.moderate`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Status filter values (active, hidden, removed) are not listed in the contract yet; adjust `communityStatuses` when they are.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

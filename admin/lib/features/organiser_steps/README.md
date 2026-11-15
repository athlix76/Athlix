# features/organiser_steps

Steps an organiser completes before approval.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `OrganiserStepsScreen` | `/organiser-steps` | list, add, edit |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/organiser-steps` | config.read |
| POST | `/admin/organiser-steps` | config.write |
| PUT | `/admin/organiser-steps/:id` | config.write |

## Permissions

View `config.read`; changes need `config.write`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- The step code is set on create only; the backend and app key off it.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

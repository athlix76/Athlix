# features/verification_requirements

Which verification each role needs, and what it gates.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `VerificationRequirementsScreen` | `/verification-requirements` | list, required switch, add |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/verification-requirements` | config.read |
| POST | `/admin/verification-requirements` | config.write |
| PUT | `/admin/verification-requirements/:id` | config.write |

## Permissions

View `config.read`; changes need `config.write`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

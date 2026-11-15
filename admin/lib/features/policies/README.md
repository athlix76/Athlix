# features/policies

Policy documents (terms, privacy, location identity notice, ...) and their versions.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `PoliciesScreen` | `/policies` | versions by type, new version editor with diff, activate |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/policies` | config.read |
| POST | `/admin/policies` | policies.write |
| POST | `/admin/policies/:id/activate` | policies.write |

## Permissions

View `config.read`; create and activate need `policies.write`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Activating a version with `requiresAcceptance` makes users accept it again. Legal text should be reviewed by a lawyer before activation.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

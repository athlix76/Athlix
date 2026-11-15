# features/plans

Premium plans and their benefits.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `PlansScreen` | `/plans` | plans, edit, benefits editor |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/plans` | config.read |
| POST | `/admin/plans` | plans.write |
| PUT | `/admin/plans/:id` | plans.write |
| PUT | `/admin/plans/:id/benefits` | plans.write |

## Permissions

View `config.read`; edits need `plans.write`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Benefit keys come from the closed list in docs/api.md; the backend rejects others.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

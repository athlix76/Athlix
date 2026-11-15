# features/fee_rules

Listing fee rules with a live preview.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `FeeRulesScreen` | `/fee-rules` | rules list, editor, live preview |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/fee-rules` | config.read |
| POST | `/admin/fee-rules` | fees.write |
| PUT | `/admin/fee-rules/:id` | fees.write |
| POST | `/admin/fee-rules/preview` | config.read |

## Permissions

View `config.read`; edits need `fees.write`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- `application/fee_formula.dart` mirrors the backend formula for display only (tested in test/features/fee_formula_test.dart). The server preview is the source of truth; if they differ, fix the mirror.
- Percentages are basis points, money is paise. No default numbers in code.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

# features/districts

Districts master list (names, LGD codes, aliases used to match DigiLocker addresses).

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `DistrictsScreen` | `/districts` | list filtered by a state dropdown, add (state picked from the list), edit, activate / deactivate |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/geo/states` | config.read (state pickers, `adminStatesProvider`) |
| GET | `/admin/geo/districts` | config.read |
| POST | `/admin/geo/districts` | geo.write |
| PUT | `/admin/geo/districts/:id` | geo.write |

## Permissions

View `config.read`; edits need `geo.write`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Districts are deactivated, never deleted: existing identities and events keep them. Aliases drive DigiLocker address matching, so check spelling before saving.
- States come from `GET /admin/geo/states` (seeded by migration, not editable here). A new district can only be added to an active state.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

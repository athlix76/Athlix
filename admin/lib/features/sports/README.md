# features/sports

Sports, positions and stat templates.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `SportsScreen` | `/sports` | sports, positions, stat template editor |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/sports` | config.read |
| POST | `/admin/sports` | sports.write |
| PUT | `/admin/sports/:id` | sports.write |
| POST | `/admin/sports/:id/positions` | sports.write |
| PUT | `/admin/positions/:id` | sports.write |
| POST | `/admin/sports/:id/stat-templates` | sports.write |
| PUT | `/admin/stat-templates/:id` | sports.write |

## Permissions

View `config.read`; edits need `sports.write`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- `adminSportsProvider` is shared with fee rules, ads and broadcast pickers.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

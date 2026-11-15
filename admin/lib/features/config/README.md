# features/config

Config editor: grouped by category, typed inputs, validation, diff before save, history.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `ConfigScreen` | `/config` | categories, search, edit dialog, history dialog |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/config` | config.read |
| PUT | `/admin/config/:key` | config.write |
| GET | `/admin/config/:key/history` | config.read |

## Permissions

View `config.read`; edit needs `config.write`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Values are parsed by `application/config_value_codec.dart` per value type (int, decimal, bool, string, string_list, json). It is unit-tested; add a test with any change.
- Saves send the `version` read; a 409 means someone saved first. Reload and redo.
- Never add a default value in code: the backend owns every number.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

# sport
The read-only sports catalogue: sports, their positions and stat templates. Serves the
public `GET /sports` and exports `Catalogue` for bootstrap and the admin sports editor
(appconfig), which owns the writes.

## Routes
| Method | Path | Group | Permission |
|---|---|---|---|
| GET | `/sports` | Public | |

## Tables
- Read only: `sports`, `sport_positions`, `stat_templates` (appconfig writes them).

## Events
None.

## Jobs
None.

## Config keys
None.

## Rules
- Public: active rows only, ordered by `sort_order`, then id. Inactive positions and
  templates are hidden even under an active sport.
- `Catalogue(ctx, q, true)` returns everything with `active` / `sortOrder` set; the public
  JSON never shows those fields.
- `ParseFields` turns bad JSON into an empty list instead of failing the whole catalogue.

## Tests
`sport_test.go`: public list (inactive filtered, children grouped by sport, no admin flags),
admin catalogue with inactive rows and flags, empty catalogue is `[]`.

# events/data

`event_models.dart`: `EventSummary` (with `category`, `eligibility`, `myEligibility`, `distanceKm`), `Event`,
`EventInput` (with `category`, `eligibility`), `EventFilters` (`category`, `showAll`, `near`, `radiusKm`,
`toQuery`), `EventContact`. `event_api.dart`: all `/events` calls including `GET /events/eligibility/preview`.

## Changing it safely

Field names match docs/api.md exactly. `EventFilters.near` is memory only. `toString()` of inputs and contact redacts.

# features/search

Global search and people search.

## Screens

| Screen | Route | Notes |
|---|---|---|
| `SearchScreen` | `/search` (bottom tab) | debounced `GET /search?q` (2+ letters), sections for people, events (`EventCard`), communities; `search` ad slot |
| `PeopleSearchScreen` | `/search/people` | `GET /search/people` with a filter sheet; cursor list |

## Filters

Basic: `q, role, sportId, positionId, state, district, city`.
Advanced: `ageMin, ageMax, achievementLevel`. Without benefit `advanced_search_filters` the server
answers 403 `PLAN_REQUIRED` and the list shows the plan-required view with a link to premium.

## Endpoints

- `GET /search?q`
- `GET /search/people?…&cursor&limit` (`limit` = `search.page_size` from config when set)

## Changing it safely

- The server already hides minors' photo, city and bio; do not try to fill them in.
- `CommunityHit` is a reduced copy of the communities model. If search needs more community
  fields, add them here, not by importing features/communities.

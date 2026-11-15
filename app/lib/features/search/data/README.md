# search/data

- `search_models.dart`: `PersonResult` (UserSummary + bio + followersCount), `CommunityHit`,
  `SearchResults`, `PeopleFilters` (with `toQuery` and `cacheKey`), `AchievementLevels`.
- `search_api.dart`: `SearchApi.all` and `SearchApi.people`, `searchApiProvider`.

Changing it safely: a new filter goes into `PeopleFilters` (field, `withQuery`, `toQuery`,
`activeCount`) and then the filter sheet. Query names must match docs/api.md.

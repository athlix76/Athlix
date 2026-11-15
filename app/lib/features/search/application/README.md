# search/application

- `search_providers.dart`: `searchResultsProvider` (family by query, empty below
  `kMinSearchLength`) and `peopleFiltersProvider` (kept while the app runs).

Changing it safely: `SearchScreen` writes `q` into `peopleFiltersProvider` before opening people
search, so keep `PeopleFilters.withQuery` copying every other field.

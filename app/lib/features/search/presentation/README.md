# search/presentation

- `search_screen.dart`: `SearchScreen`.
- `people_search_screen.dart`: `PeopleSearchScreen` and its private filter sheet.

Changing it safely: the people list is keyed by `PeopleFilters.cacheKey`; any change of filters
must produce a new key or the list will not reload.

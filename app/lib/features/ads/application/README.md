# ads/application

- `ad_providers.dart`: `adsProvider`, an auto-dispose family keyed by `AdQuery`. Any error
  becomes an empty list so an ad never breaks the screen around it.

Changing it safely: keep the family key a value type (a record) so equal queries share one request.

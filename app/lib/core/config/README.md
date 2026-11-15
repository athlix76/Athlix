# core/config

Server-driven configuration.

- `bootstrap.dart`: the `GET /bootstrap` model (public config, sports, min versions, maintenance).
- `app_config.dart`: `AppConfig`, typed getters for config keys (`event.max_search_radius_km`,
  `location.enabled`, `listing.order_ttl_minutes`, ...). Getters return null when a key is missing.
- `config_providers.dart`: bootstrap controller, `appConfigProvider`, `sportsProvider`, `appGateProvider`.
- `version.dart`: `compareVersions`, `needsForceUpdate`.

## Changing it safely

Add a getter for a new key using its exact name from docs/config-system.md, and handle null (key absent)
with a safe UI default rather than a business number. Never put secrets in config.

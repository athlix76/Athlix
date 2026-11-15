# home/presentation

- `home_shell.dart`: `HomeShell(navigationShell:)`, the five-tab bottom navigation.
- `home_screen.dart`: `HomeScreen`, built only from other features' providers and widgets
  (events, ads); it owns no data of its own.

Changing it safely: quick links are a list of `(icon, label, route)` records in `_QuickLinks`;
use `Routes.*` constants, never literal paths.

# lib

Dart source of the admin panel.

| Path | What it is |
|---|---|
| `main.dart` | Entry point: `ProviderScope` + `AdminApp`. |
| `app.dart` | `MaterialApp.router` and theme. Idle sign-out lives in `core/widgets/admin_shell.dart`. |
| `core/` | Shared code (API client, auth, router, widgets). See `core/README.md`. |
| `features/` | One folder per area of the panel. See `features/README.md`. |

## Changing it safely

- Keep the feature-first layout: a new screen goes in a feature folder, not in `core/`.
- No `print` / `debugPrint` of data (the `avoid_print` lint is on). Never log tokens, PII or bodies.

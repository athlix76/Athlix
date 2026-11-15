# app/lib

All Dart source of the app.

- `main.dart`: `ProviderScope` + `AthlixApp`. Keep it tiny.
- `app.dart`: `MaterialApp.router`, themes, and the listener that starts push after sign-in.
- `core/`: shared plumbing with no feature knowledge (one exception: the router lists every screen).
- `features/`: one folder per product area, each split into `data/`, `application/`, `presentation/`.

## Changing it safely

Features may import `core/` and another feature's `data/` or a public widget; `core/` must not import
features except from `core/router`. Keep secrets, tokens and PII out of logs and `toString()`.

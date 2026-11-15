# features/startup

Gate screens shown before or instead of the app: splash (bootstrap and session loading), force update, maintenance and suspended account.

## Endpoints

- `GET /bootstrap (via core/config)`

## Rules

- No business logic here; `computeRedirect` chooses the screen.

## Changing it safely

Change gate order only in `core/router/redirect.dart` and its test.

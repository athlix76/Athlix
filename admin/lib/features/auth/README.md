# features/auth

Sign-in, session, sign-out and changing your own password.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `LoginScreen` | `/login` | email + password + TOTP code |
| `showChangePasswordDialog` | dialog (key icon in the top bar) | current password, new password twice, TOTP code; other sessions are signed out, this one switches to the returned tokens |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| POST | `/admin/auth/login` | public |
| POST | `/admin/auth/refresh` | public (called by core/api/api_client.dart) |
| POST | `/admin/auth/logout` | any |
| GET | `/admin/me` | any |
| POST | `/admin/me/password` | any |

## Permissions

None to sign in. `SessionController.can()` is what every other screen uses for gating.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Tokens live only in `core/auth/token_store.dart` (memory). Never add localStorage, sessionStorage or cookies.
- Never print the login body, the TOTP code or a token. `TokenPair.toString()` is redacted on purpose.
- Idle sign-out is `IDLE_TIMEOUT_MINUTES` (dart-define, default 15) via `core/auth/idle_watcher.dart`.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

# core/session

`session_controller.dart`: `SessionState` (`unknown`, `signedOut`, `signedIn` + `Me`), `SessionController`
(load `GET /me`, sign in / out, react to API signals), `sessionProvider`, `meProvider`, `currentUserIdProvider`.

## Changing it safely

Sign-out must clear tokens through the token store and stop push first. Never log `Me`.

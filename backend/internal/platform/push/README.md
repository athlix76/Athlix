# platform/push

Mobile push notifications through Firebase Cloud Messaging (HTTP v1 API).

## API

| Name | Use |
|---|---|
| `Sender` | `Send(ctx, Message) error`: one push to one device token |
| `Message` | `{Token, Title, Body, Data map[string]string}`. Data holds deep-link ids only |
| `NewFCM(Credentials, ...Option)` | Production sender. Options: `WithTokenURL`, `WithSendBaseURL`, `WithHTTPClient`, `WithClock` (tests) |
| `FromSecrets(ctx, getter, name)` | Reads the `fcm` secret (`cfg.SecretName(app.SecretFCM)`) and builds the FCM sender |
| `Nop` | Returns `ErrDisabled` (push not configured) |
| `Fake` | Records `Sent` messages; `Fail[token]` returns an error for that token. For tests in any package |
| `ErrUnregistered` | The token is dead (404, `UNREGISTERED`, `SENDER_ID_MISMATCH`, or an invalid `message.token`). The caller deletes the token row |
| `ErrDisabled` | Push is off in this environment; skip quietly |

## How it works

1. The `fcm` secret (`{"projectId","clientEmail","privateKey"}`, see docs/security.md) gives a
   service-account key. PKCS#8 and PKCS#1 PEM are accepted.
2. A JWT signed with RS256 (`iss` = client email, `scope` = firebase.messaging, `aud` = token
   URL, 1 h lifetime) is exchanged at `https://oauth2.googleapis.com/token` for an access token.
3. The access token is cached **in memory only** until 60 s before expiry. A 401 from FCM drops
   it and retries once.
4. `POST https://fcm.googleapis.com/v1/projects/<projectId>/messages:send` with
   `notification`, `data`, Android high priority and the default APNs sound.

The HTTP client has a 10 s timeout.

## Rules

- Never log or return device tokens, access tokens, the signed JWT, the private key or FCM
  response bodies. Errors are generic, e.g. `push: fcm send failed (status 503)`.
- Push text uses display names and titles only, never mobile numbers or other PII.
- The key lives only in Secrets Manager; nothing here writes it to disk, Redis or env vars.

## Tests

`push_test.go` runs against an httptest server:

- the token is fetched once and reused, then fetched again after expiry;
- a 401 triggers one refresh;
- the error mapping to `ErrUnregistered`;
- error strings contain no token or key;
- the PEM formats;
- `FromSecrets`, `Fake` and `Nop`.

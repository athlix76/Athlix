# cmd/ws

The Lambda behind the API Gateway **WebSocket API**. `config.Load()` → `app.Bootstrap()` →
`chat.NewService(d)` (no `modules.All`, no router). It handles transport only; the protocol
lives in the [chat module](../../internal/modules/chat/README.md).

## Routes

| Route key | What happens |
|---|---|
| `$connect` | `?ticket=` is redeemed once (`chat.ConsumeTicket`); 401 when missing, used, expired, or the user may not chat. On success the connection is stored (`ws:conn:<connId>`, `ws:user:<userId>`). |
| `$disconnect` | The connection is removed from Redis (idempotent). |
| `$default`, `sendMessage`, `typing`, `markRead`, `ping` | The frame goes to `chat.HandleFrame`; its replies (`ack`, `error`, `pong`) are posted back to the same connection. An unknown connection gets an `UNAUTHENTICATED` error frame. |

Every route returns 200 except a refused or failed `$connect`.

## Tables

None directly; see the chat module.

## Events

`message.created` is published by `chat.SendMessage`.

## Jobs

None.

## Config keys

- `WS_MANAGEMENT_URL`: `https://<api-id>.execute-api.<region>.amazonaws.com/<stage>`. If unset,
  it is derived from the first request's domain and stage, which only works on the default
  execute-api domain.
- Chat settings: see the chat module (`chat.ws_ticket_ttl_seconds` and others).
- IAM: `execute-api:ManageConnections` on this API's connections only.

## Rules

- Outside Lambda (`AWS_LAMBDA_FUNCTION_NAME` empty) it prints where the local WebSocket is
  (`/v1/chat/ws` on cmd/api with `ENV=local`) and exits 0.
- Tickets, tokens and message bodies are never logged; logs carry the route key, request id
  and user id.
- Build with `go build -tags lambda.norpc ./cmd/ws/` for the `provided.al2023` runtime.

## Tests

The logic is tested in the chat module (`HandleFrame`, tickets, connection registry). Check this
command with `go build ./cmd/ws/ && go vet ./cmd/ws/`.

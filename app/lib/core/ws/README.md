# core/ws

- `chat_socket.dart`: `ChatSocket` over web_socket_channel. Connects with a one-time ticket from
  `POST /chat/ws-ticket` (never the access token), parses `ServerFrame`s, sends typed frames (`WsType`).
- `backoff.dart`: capped exponential reconnect delay.

## Changing it safely

Get a fresh ticket for every connect. Never log frames (they hold message text).

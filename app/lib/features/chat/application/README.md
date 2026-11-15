# features/chat/application

Live state for chat: the socket and the open thread.

## Contents

- `chat_socket_provider.dart`:
  - `chatSocketProvider`: one `ChatSocket` per signed-in user, with tickets from `POST /chat/ws-ticket`.
  - `chatSocketStatusProvider`: the socket's connection status.
  - `conversationProvider`: a conversation by id.
  - `ConversationController`: mute and leave.
- `thread_controller.dart`: `threadControllerProvider(conversationId)`, which handles:
  - history (newest page first, then `before=<oldest id>` for older pages)
  - optimistic sends keyed by `clientMsgId`. A send goes over the socket, and falls back to REST
    when there is no `ack` within `kAckTimeout` or the socket is down. This is safe because the
    REST send is idempotent.
  - image sends (upload as `chat_image` first)
  - delete
  - read receipts (`markRead` plus server `read` frames)
  - typing (sending is throttled, and the indicator clears after `kTypingTtl`)
  - polling every `kPollInterval` while the socket is down
  - a refresh on app resume and on reconnect
- `client_msg_id.dart`: UUID v4 generator built on `Random.secure()`.

## Changing it safely

- Timings and page size are the constants at the top of `thread_controller.dart`. They are
  transport details, not product rules: product limits come from public config.
- Every new socket path also needs a REST path.
- Never log frames, tickets or message bodies.
- `ThreadState.notice` is a new object each time, so the screen shows every notice once, even
  when two notices have the same text.

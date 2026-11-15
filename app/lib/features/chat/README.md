# features/chat

Direct messages, community chats and event group chats (docs/api.md "Chat"). One screen set
serves all three conversation types.

## Screens

| Screen | Route | Notes |
|---|---|---|
| `ConversationsScreen` | `/chat` (pushed from the chat icon in the Home app bar) | all conversations, unread badges; reloads (debounced) on `message.new` / `message.deleted` frames and on app resume; link to Communities |
| `ChatThreadScreen` | `/chat/:id` | messages with optimistic send, retry/discard, reply, delete, report, images (when `chat.media_enabled`), typing line, "Seen" receipts; menu: profile / community / event, members, mute, leave, report |
| `ChatMembersScreen` | `/chat/:id/members` | read-only member list with roles |

## Endpoints

- `POST /chat/ws-ticket` (socket ticket), `GET /chat/conversations`, `POST /chat/direct`
- `GET /chat/conversations/:id`, `GET /chat/conversations/:id/messages?before&limit`
- `POST /chat/conversations/:id/messages`, `DELETE /chat/messages/:id`
- `POST /chat/conversations/:id/read|mute|leave`, `GET /chat/conversations/:id/members`
- `GET /events/:id/group`
- `POST /media/uploads` + confirm (via `core/media`, purpose `chat_image`)

## Rules

- The app works without the WebSocket: every send, read receipt and refresh has a REST path.
- Message bodies, image URLs and tickets are never logged. `Message.toString()` leaves the body
  out and `WsTicket.toString()` redacts.
- The message length limit comes from `chat.message_max_length`. The image button appears only
  when `chat.media_enabled` is on. The server checks both again.
- `DELETE` and `leave` rights are enforced by the server; the UI only hides what can't work.

## Changing it safely

Keep `Message.fromJson` and `Message.toJson` symmetric (a test round-trips them). For a new
server frame, add it to `core/ws/chat_socket.dart` `WsType` first, then handle it in
`ThreadController._onFrame`.

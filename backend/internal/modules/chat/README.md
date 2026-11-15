# chat

Conversations (`direct`, `community`, `event`), their members and messages. Serves the chat
REST API, the WebSocket protocol (shared by the local route and [cmd/ws](../../../cmd/ws/README.md)),
fan-out to online members, and event-group membership driven by event and registration events.
Community groups are created and managed by the `community` module; this module only stores and
delivers their messages.

## Routes

Contract: [docs/api.md "Chat"](../../../../docs/api.md) and "WebSocket protocol".

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/chat/ws-ticket` | Verified | any verified user |
| GET | `/chat/conversations?cursor&limit` | Verified | own conversations only |
| GET | `/chat/unread-count` | Verified | own active, unmuted conversations: `{conversations, messages}` |
| POST | `/chat/direct` `{userId}` | Verified | DM rules below, else 403 |
| GET | `/chat/conversations/:id` | Verified | active member, else 404 |
| GET | `/chat/conversations/:id/messages?before&limit` | Verified | active member, else 404 |
| POST | `/chat/conversations/:id/messages` | Verified | active member, else 404 (returns 201) |
| POST | `/chat/conversations/:id/read` `{messageId}` | Verified | active member, else 404 |
| POST | `/chat/conversations/:id/mute` `{mutedUntil}` | Verified | active member, else 404 |
| POST | `/chat/conversations/:id/leave` | Verified | active member, else 404; not direct chats, not the owner |
| GET | `/chat/conversations/:id/members?cursor&limit` | Verified | active member, else 404 |
| DELETE | `/chat/messages/:id` | Verified | the sender, or an owner/admin of a group, else 404 |
| GET | `/events/:id/group` | Verified | active member of the event group, else 404 |
| GET | `/admin/conversations/:id/messages` | Admin | `moderation.read_messages`; 404 unless an open report exists on the conversation or one of its messages; every read is audited |
| GET | `/v1/chat/ws?ticket=` | Engine (`ENV=local` only) | the single-use ticket |

## Tables

- Owned: `conversations`, `conversation_members`, `messages`.
- Read only: `reports` (admin gate), `user_settings.privacy` (`allowDmsFrom`),
  `consultation_requests` (`accepted`), `events`.
- Through platform services: `users`, `blocks`, `follows`, `profiles` (`d.Users`),
  `media_uploads` (`d.Media.Claim`, `d.Media.ViewURLs`), `admin_audit_log` (`audit.Admin`).

## Redis

| Key | Value | TTL |
|---|---|---|
| `ws:ticket:<t>` | userId | `chat.ws_ticket_ttl_seconds`; redeemed once with `GETDEL` |
| `ws:conn:<connId>` | userId | 2 h, refreshed on `ping` |
| `ws:user:<userId>` | SET of connIds | 2 h, refreshed on `ping` |
| `rl:chat:<userId>` | send counter | 1 minute |

The `notification` module reads `ws:user:<userId>` to decide whether a user is online.

## Events

- Published after commit: `message.created` (`events.MessagePayload`), only for a new message
  (not for a `clientMsgId` retry).
- Consumed (all idempotent: unique keys, status-guarded updates, `member_count` is recounted):
  - `event.published`: create the event group (organiser as owner) when `event.auto_create_group`.
  - `registration.created` (status `registered`) and `registration.promoted`: add the player,
    unless the group is not active, the player is banned or already in, or the group is full.
  - `registration.withdrawn`: mark the player as left.
  - `event.cancelled`, `event.completed`: archive the event group.
  - `user.deleted`: mark every visible message the user sent as `deleted` (shown as removed, no
    body), as docs/security.md promises for account deletion.

## Jobs

None.

## Config keys

| Key | Default | Use |
|---|---|---|
| `chat.message_max_length` | `2000` | Maximum characters (runes) in a message body |
| `chat.messages_per_minute` | `30` | Sends per user per minute (REST and WebSocket together) |
| `chat.media_enabled` | `true` | When false, image messages are rejected (422 on `kind`) |
| `chat.dm_requires_follow` | `false` | When true, `allowDmsFrom: everyone` is treated as `followers` |
| `chat.ws_ticket_ttl_seconds` | `60` | Lifetime of a WebSocket ticket |
| `chat.group_max_members` | `500` | Event-group size cap; extra registrations are not added |
| `minor.dm_policy` | `"mutual_follow"` | DMs involving a minor: `none`, `mutual_follow` or `anyone` |
| `event.auto_create_group` | `true` | Create an event group on `event.published` |
| `app.maintenance_mode` | `false` | Refuses tickets and WebSocket sends, like the Verified group |

Environment: `WS_PUBLIC_URL` (returned with each ticket) and `WS_MANAGEMENT_URL`. When
`WS_MANAGEMENT_URL` is set and AWS is configured, fan-out uses the API Gateway Management API;
otherwise it uses the in-process hub of the local route. The **api** Lambda needs
`WS_MANAGEMENT_URL` too, or REST sends are not pushed to online members.

## Rules

- The caller always comes from the token (`authn.UserID` / `authn.User`) or from the ticket,
  never from the request.
- Membership is checked in the query (`m.user_id = ? AND m.status = 'active'`); a non-member
  gets 404, the same as a missing conversation.
- DM rules (all must pass): not blocked either way; recipient active and onboarding done; if
  either user is a minor, `minor.dm_policy`; the recipient's `allowDmsFrom` (`everyone`,
  `followers`, `mutual`, `nobody`, raised to `followers` by `chat.dm_requires_follow`). For two
  adults an accepted consultation passes the `allowDmsFrom` check. Sends in a direct chat
  re-check blocks.
- `SendMessage` is the one send path (REST, local WebSocket, cmd/ws). `clientMsgId` is required;
  the same sender and `clientMsgId` returns the existing message without fan-out or event, and
  reusing it in another conversation is 409.
- Deleted and hidden messages are returned without body or image; only the audited admin
  route shows them in full.
- Delivery failures never fail a send. Connections reported gone are removed from Redis.
- Never log tickets, tokens or message bodies; log ids only.

## Tests

`go test ./internal/modules/chat/` (sqlmock + miniredis, fake `Delivery`). Covers: send happy
path with fan-out and gone-connection cleanup, image send, media disabled, length and rate
limits following setting changes, `clientMsgId` idempotency and conflict, blocked and closed
chats, 404 for non-members on every conversation route, list paging, delete / read / leave,
every DM rule case (including minor `mutual_follow`, the consultation exception and
`chat.dm_requires_follow`), direct create / refuse, ticket single use and TTL from the setting,
connection registry and ping, WebSocket frames (ack skips the origin connection, errors,
typing), the local WebSocket route end to end, event handlers dispatched twice, and the admin
route (403 without the permission, 404 without an open report, audited happy path).

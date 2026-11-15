# Chat: ticket, connect, send, fan-out, push

Messages are saved in MySQL first, then pushed over WebSocket to members who are online, then
the `message.created` event makes the notification module push to members who are offline and
have not muted the conversation. The same send function is used by REST, the local WebSocket
route and the `ws` Lambda.

In AWS the socket is an **API Gateway WebSocket API** routed to the `ws` Lambda (`$connect`,
`$disconnect`, `sendMessage`, `typing`, `markRead`, `ping`). Locally `cmd/api` serves
`GET /chat/ws` with an in-memory hub and the same frames.

## Connect and send

```mermaid
sequenceDiagram
    autonumber
    actor S as Sender
    participant App as Sender app
    participant API as Athlix API
    participant GW as API Gateway WebSocket
    participant WS as WS Lambda
    participant R as Redis
    participant DB as MySQL
    participant Q as SQS athlix-events
    participant W as Worker
    participant B as Recipient app online
    participant FCM as FCM

    App->>API: POST /chat/ws-ticket
    API->>R: Store ws ticket to userId for chat.ws_ticket_ttl_seconds, single use
    API-->>App: {ticket, url, expiresAt}
    App->>GW: Connect url?ticket=...
    GW->>WS: $connect
    WS->>R: Redeem ticket, store ws conn to userId and add to ws user set
    WS-->>GW: 200 accepted

    S->>App: Type and send
    App->>GW: {"action": "sendMessage", "data": {conversationId, clientMsgId, kind, body}}
    GW->>WS: sendMessage
    WS->>DB: Check membership and conversation status
    WS->>R: Rate limit chat.messages_per_minute
    alt Not a member
        WS-->>App: {"type": "error", "data": {clientMsgId, code NOT_FOUND}}
    else Rate limited or too long
        WS-->>App: {"type": "error", "data": {clientMsgId, code RATE_LIMITED or VALIDATION_FAILED}}
    else OK
        WS->>DB: Insert message, idempotent on clientMsgId
        WS-->>App: {"type": "ack", "data": {clientMsgId, message}}
        WS->>R: Look up online connections of every member
        WS->>GW: Post message.new to each connection via the Management API
        GW-->>B: {"type": "message.new", "data": Message}
        Note over WS,GW: 410 Gone connections are removed from Redis
        WS->>Q: Publish message.created after commit
        Q->>W: message.created
        W->>W: Members with no connection, not muted, notificationPrefs.messages on
        W->>FCM: Push with display name only, never PII
    end
```

- `clientMsgId` makes sending safe to retry: the same id returns the same message.
- Limits: `chat.message_max_length`, `chat.messages_per_minute` (429), images only when
  `chat.media_enabled` and with a `chat_image` upload (`mediaUploadId`).
- Other frames: `{"action": "typing"}` → `{"type": "typing"}` to members;
  `{"action": "markRead"}` → `{"type": "read"}`; deletes fan out `{"type": "message.deleted"}`.

## Keep-alive and fallback

```mermaid
sequenceDiagram
    autonumber
    participant App as Athlix app
    participant GW as API Gateway WebSocket
    participant WS as WS Lambda
    participant API as Athlix API

    loop Every 5 minutes
        App->>GW: {"action": "ping"}
        GW->>WS: ping, refreshes the connection TTL
        WS-->>App: {"type": "pong"}
    end
    GW-->>App: Socket closed, idle 10 minutes or network change
    App->>API: POST /chat/ws-ticket for a new ticket, then reconnect
    App->>API: GET /chat/conversations/:id/messages?before=messageId on resume
    App->>API: POST /chat/conversations/:id/messages when the socket is down
    API-->>App: 201 Message
```

The app must work without the socket: REST send, and poll `GET .../messages` on resume.

## Who can start a DM

`POST /chat/direct {userId}` (and `Profile.canMessage`) passes only if all hold:

1. Not blocked either way; the recipient finished onboarding and is active.
2. The recipient's `privacy.allowDmsFrom` (`everyone`, `followers`, `mutual`, `nobody`); with
   `chat.dm_requires_follow` at least `followers`.
3. If either person is a minor, `minor.dm_policy` (`none`, `mutual_follow`, `anyone`).
4. For two adults, an `accepted` consultation passes rule 2.

Failure: 403 `FORBIDDEN` "You can't message this person." Event groups are created on
`event.published` (if `event.auto_create_group`) and membership follows
`registration.created`, `registration.withdrawn` and `registration.promoted`.

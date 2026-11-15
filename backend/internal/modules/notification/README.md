# notification

Turns domain events into in-app notifications and FCM pushes. Owns device tokens, the
notification list and read state, the admin broadcast, and the hourly event reminders.
Push goes through [platform/push](../../platform/push/README.md).

## Routes

Sections in [docs/api.md](../../../../docs/api.md): "Devices", "Notifications", and admin
"Communities, messages, broadcast".

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/v1/notifications/devices` | user | – |
| DELETE | `/v1/notifications/devices` | user | – |
| GET | `/v1/notifications` | verified | – |
| GET | `/v1/notifications/unread-count` | verified | – (`{unreadCount}`, the bell badge) |
| POST | `/v1/notifications/read` | verified | – |
| POST | `/v1/admin/notifications/broadcast` | admin | `notifications.broadcast` |

## Tables

- **Owned:** `notifications`, `device_tokens`.
- **Read only:**
  - `user_settings.notification_prefs`
  - `users`, `profiles`, `user_roles` (broadcast audience)
  - `events`, `event_registrations` (titles, recipients, reminders)
  - `conversations`, `conversation_members`, `messages` (chat push)
  - display names via `d.Users`

## Events

- **Consumed:**
  - verification.completed, guardian.consent_granted
  - guardian.consent_revoked (tells the minor consent was withdrawn)
  - follow.created
  - organiser.approved / .rejected
  - event.submitted / .published / .rejected / .cancelled / .updated
  - registration.created / .withdrawn / .promoted / .payment_marked
  - listing.paid / .refunded
  - message.created
  - post.liked / .commented
  - consultation.requested / .responded
  - subscription.activated / .expired
  - community.join_requested / .join_approved
  - user.deleted: delete all of the user's `device_tokens`, so the deleted account's phones get
    no more pushes (docs/security.md, account deletion)
  - `notification.broadcast_push` (internal, see below)
- **Published:** `notification.broadcast_push` `{firstId, lastId, afterId}`, only to itself.
  It pushes one broadcast in batches of 500 from the worker, then queues the next batch.

Who receives what:

| Event | Recipient | Category |
|---|---|---|
| verification / guardian / subscription | the user | account (always pushed) |
| follow.created | followee | social |
| post.liked / .commented | post author (not when acting on your own post) | social |
| consultation.requested | coach | social |
| consultation.responded | the other party of the actor (player by default) | social |
| community.join_requested | owner | social |
| community.join_approved | requester | social |
| organiser.* | the user | events |
| event.submitted / .published / .rejected | organiser | events |
| event.cancelled | registered and waitlisted players, plus the organiser when the event was `removed` by an admin | events |
| event.updated | registered and waitlisted players | events |
| registration.created / .withdrawn | organiser | events |
| registration.promoted / .payment_marked | player | events |
| listing.* | organiser | events |
| message.created | active, unmuted, offline members except the sender. **Push only, no in-app row** | messages |
| broadcast | audience | marketing (default off) |
| event.reminder (job) | registered players | events |
| location_identity.changed (verified / rejected / expired / revoked; pending, superseded and withdrawn are silent) | the user; data `{"screen":"location_identity"}` plus `rejectionCode` when rejected | events |
| location_identity.expiring | the user (expiry date shown in IST) | events |
| registration.eligibility_flagged | organiser (data `screen: event_registrations`, eventId, userId) and player (`screen: location_identity`, eventId); text says it is not cancelled | events |

## Jobs

`notification.event_reminders` runs hourly:

- It selects registrations with status `registered` for `published` events starting within
  (now, now + `notification.event_reminder_hours`].
- It reads them in batches of 500 by registration id, with at most 20 batches per run.
- Each reminder is deduped with Redis `notif:reminder:<eventId>:<userId>`, TTL = hours + 48 h.

## Config keys

`notification.event_reminder_hours` (int, default 24, same as the seed).

## Rules

- Every event writes at most one row per recipient. Redis `SET NX`
  `notif:evt:<sha256(type|payload|occurredAt)>:<userId>` (TTL 72 h) guards SQS redelivery.
  If the insert fails, the key is deleted and the error is returned, so the retry redoes it.
- In-app rows are always written. A push is sent only when the user's category in
  `notificationPrefs` is on (defaults: messages, events and social on; marketing off). Account
  notices have no category and are always pushed.
- Push failures never fail a handler once rows exist. Dead tokens (`push.ErrUnregistered`) are
  deleted.
- `message.created` claims per recipient. If prefs or tokens fail to load, it releases the
  claims and returns the error so the event is retried.
- "Online" means the chat-owned Redis set `ws:user:<userId>` has members.
- Text uses display names and event/community titles only. It never contains mobile numbers,
  other PII, or the chat message text.
- The FCM sender is built lazily from the `fcm` secret. If the secret is missing, push is off
  for 5 minutes at a time and this is logged without detail. Device tokens are never logged.
- Broadcast:
  - Rows and the `admin_audit_log` entry are written in one transaction.
  - The audit details hold the count and filters only.
  - Minors are excluded unless `includeMinors` is set.
- Read and delete queries are scoped by the caller id from the token
  (`WHERE ... user_id = ?`).

## Tests

`notification_test.go` covers:

- Devices: upsert and delete, scoped to the caller, plus validation.
- Notification list: page, cursor, `unreadCount`, null `readAt`.
- Mark read: by ids or all, scoped to the user, with 422 cases.
- Every mapped event has a handler.
- follow.created, including idempotent redelivery.
- Prefs off: the row is written but nothing is pushed.
- Account notices: no prefs lookup, and dead tokens are deleted.
- A failed insert releases the dedupe key so the retry works.
- event.cancelled for players and a removed event's organiser; registration.created; and
  community.join_requested.
- message.created: online and offline, muted members excluded by the query, direct and group
  text, and no duplicate on redelivery.
- Push is disabled when there is no secret.
- Broadcast: happy path with the push batch, zero recipients, 403 without the permission,
  validation, and paging with dedupe.
- Reminder job: window, dedupe, the window following the config value, and the TTL.

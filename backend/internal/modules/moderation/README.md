# moderation

User reports and admin moderation. Verified users report a user, event, post, comment, message
or conversation; heavily reported posts, comments and messages are hidden automatically; admins
work the report queue and can act on any target directly. This module may update ONLY the
`status` column of `users`, `events`, `posts`, `post_comments`, `messages` and `conversations`
(docs/architecture.md exception 1).

## Routes

See [docs/api.md](../../../../docs/api.md) sections "Reports" and "Admin panel → moderation".

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/reports` | verified | |
| GET | `/admin/reports?status&targetType&reason&cursor&limit` | admin | `reports.read` |
| POST | `/admin/reports/:id/resolve` | admin | `reports.resolve` |
| GET | `/admin/moderation/actions?targetType&targetId&cursor&limit` | admin | `reports.read` |
| POST | `/admin/moderation/actions` | admin | `reports.resolve` |

## Tables

- Owned: `reports`, `moderation_actions`.
- Report targets: `user`, `event`, `post`, `comment`, `message`, `conversation`, `community`
  (a community is reported as itself; its id is the conversation id).
- Status column only: `users`, `events`, `posts`, `post_comments`, `messages`, `conversations`.
- Read: `profiles`, `follows`, `blocks`, `conversation_members`, `admin_users` (action author name),
  `admin_audit_log` (written through `platform/audit`).

## Events

- Published: `report.created` (after commit, new reports only), `event.cancelled` when an admin
  hides or removes an event (reason "Removed by Athlix moderation."; the admin note stays internal).
- Consumed: none.

## Jobs

None.

## Config keys

| Key | Default | Use |
|---|---|---|
| `moderation.auto_hide_report_threshold` | `5` | open reports on a post / comment / message that hide it; below 1 disables auto-hide |
| `moderation.report_rate_limit_per_hour` | `20` | reports one user may file per hour (429 above it) |

## Rules

- A report needs a target the reporter can currently see (same visibility rules as the owning
  module); otherwise 404. Reporting a user who is blocked either way is also 404. Self-report → 422.
- One report per reporter and target: a repeat returns 200 with the existing id (also on the
  insert race). New reports return 201.
- Rate limit: 20 reports per user per hour (`reportRateLimit`, no config key) → 429.
- Auto-hide applies only to posts, comments and messages (visible → hidden) and records an
  `auto_hide` action with no admin. Users, events and conversations are only queued.
- Queue order: `underage_safety` first, then newest; keyset cursor keeps the two buckets apart.
- Message text in the queue is shown only with `moderation.read_messages`, and that read is
  audited with the message ids.
- Status changes per target: post / comment / message hide ↔ unhide (visible / hidden), remove →
  deleted; conversation hide / remove → removed, unhide → active; event hide / remove → removed
  (from published / pending_review), unhide not supported; user hide / remove / suspend →
  suspended, unhide / unsuspend → active. Deleted users are never changed. An action on an item
  already in the target status is recorded as a no-op; any other status → 409.
- `suspend_user` / `unsuspend_user` on content acts on, and is recorded against, its owner
  (no owner → 409).
- Resolving a report with an action marks every open report on that target as actioned; a
  dismissal marks only that report and lifts an auto-hide once open reports fall below the
  threshold and no admin has acted since. Direct hide / remove / suspend also action open reports.
- Every admin write runs in one transaction with `audit.Admin`; user caches are invalidated and
  events are published after commit. Logs carry ids only.

## Tests

`moderation_test.go`: create + `report.created`, duplicate and insert race → 200, auto-hide at
the threshold and idempotency, default threshold, user targets queued only, 404 for hidden /
blocked / draft / private targets, 422 validation, 429 rate limit, admin permission 403,
resolve hide / remove event (`event.cancelled`) / suspend owner / dismiss lifting auto-hide,
resolve 404 / 409 / 422, queue paging with the underage bucket, message text hidden vs audited,
direct actions (exact response, report closing, unsupported / missing / ownerless → 409 / 404),
action history paging.

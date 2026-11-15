# Moderation

Any verified user can report a user, event, post, comment, message or conversation. Content
auto-hides when enough distinct people report it, and admins work a queue with
`underage_safety` reports first. Reading the messages of a reported conversation needs a
separate permission, is allowed only while a report is open, and is audited.

## Report and auto-hide

```mermaid
sequenceDiagram
    autonumber
    actor U as Reporter
    participant App as Athlix app
    participant API as Athlix API
    participant DB as MySQL

    U->>App: Report with a reason
    App->>API: POST /reports {targetType, targetId, reason, details}
    API->>DB: Check the reporter can see the target
    alt Target not visible to the reporter
        API-->>App: 404 NOT_FOUND
    else Same reporter already reported this target
        API-->>App: 200 {id} of the existing report
    else New report
        API->>DB: Insert report with status open
        API-->>App: 201 {id}
        Note over API: emits report.created
        opt Distinct open reports reach moderation.auto_hide_report_threshold
            API->>DB: Set target status hidden and record an automatic moderation action
        end
    end
```

`reason`: `spam`, `fake_profile`, `harassment`, `scam`, `inappropriate`, `underage_safety`,
`payment_issue`, `other`. Reports are rate limited (`report:<userId>`, 20 per hour).
Auto-hidden content is invisible to everyone except moderators until an admin decides.

## Admin review

```mermaid
sequenceDiagram
    autonumber
    actor A as Moderator
    participant AP as Admin panel
    participant API as Athlix API
    participant DB as MySQL

    AP->>API: GET /admin/reports?status=open
    API-->>AP: Page of AdminReport, underage_safety first, with targetPreview and openReportsOnTarget
    opt Target is a message or conversation
        A->>AP: Open messages and type a reason
        AP->>API: GET /admin/conversations/:id/messages?before
        alt No open report on the conversation or its messages
            API-->>AP: 404 NOT_FOUND
        else Open report
            Note over API: needs moderation.read_messages, view written to admin_audit_log
            API-->>AP: Page of Message
        end
    end
    AP->>API: GET /admin/moderation/actions?targetType&targetId
    API-->>AP: Earlier actions on this target
    A->>AP: Choose an action and write a note, confirm
    AP->>API: POST /admin/reports/:id/resolve {action, note}
    Note over API: dismiss, hide, remove, suspend_user or warn
    API->>DB: Update target status, insert moderation_actions, report actioned or dismissed, admin_audit_log in one transaction
    API-->>AP: AdminReport
```

- Permissions: `reports.read` (queue, history), `reports.resolve` (resolve, direct actions),
  `moderation.read_messages` (reported conversations), `communities.moderate`
  (`GET /admin/communities`, `POST /admin/communities/:id/remove`), `users.write`
  (`POST /admin/users/:id/suspend`, `/unsuspend`).
- Direct action without a report: `POST /admin/moderation/actions {targetType, targetId,
  action, reason}` with `hide`, `unhide`, `remove`, `suspend_user`, `unsuspend_user`, `warn`.
- `moderation` is the only module allowed to change the `status` column of `users`, `events`,
  `posts`, `post_comments`, `messages` and `conversations`; owners treat `hidden`, `removed`
  and `suspended` as invisible.
- A suspended user gets 403 `ACCOUNT_SUSPENDED` on every app route.
- Removing a published event uses `POST /admin/events/:id/remove` (no listing refund; see
  [tournament-listing.md](tournament-listing.md)).

## Report status

```mermaid
stateDiagram-v2
    [*] --> open: POST /reports
    open --> actioned: resolve with hide, remove, suspend_user or warn
    open --> dismissed: resolve with dismiss
    actioned --> [*]
    dismissed --> [*]
```

## Content status

```mermaid
stateDiagram-v2
    [*] --> visible
    visible --> hidden: auto-hide threshold or admin hide
    hidden --> visible: admin unhide
    visible --> removed: admin remove
    hidden --> removed: admin remove
    removed --> [*]
```

Whether dismissing a report automatically un-hides auto-hidden content is a moderation module
rule; check its README. If not, the moderator uses `unhide` explicitly.

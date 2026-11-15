# features/moderation

Moderation action history, manual actions, and reading reported conversations.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `ModerationScreen` | `/moderation` | action history with filters, new action, open a reported conversation |
| `ConversationMessagesScreen` | `/moderation/conversations/:id` | messages of a reported conversation, after a reason prompt |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/moderation/actions` | reports.read |
| POST | `/admin/moderation/actions` | reports.resolve |
| GET | `/admin/conversations/:id/messages` | moderation.read_messages (audited; only with an open report) |

## Permissions

Menu item: `reports.read` or `moderation.read_messages`. History needs `reports.read`; actions need `reports.resolve`; messages need `moderation.read_messages`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- The read reason is passed in memory (route `extra`) and sent as `?reason=`; it never goes into the browser URL. A reload asks again.
- Never log message text.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.

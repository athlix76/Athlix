# features/moderation/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `conversation_messages_screen.dart` | Asks for the reason, then opens the reported conversation. |
| `moderation_screen.dart` | Moderation action history (manual and automatic), a form to act on any target, and the entry point for reading a reported conversation. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.

# features/chat/presentation

Chat screens. They hold UI state only. Data and transport live in `../application`.

## Contents

- `conversations_screen.dart`: `ConversationsScreen` and the reusable `ConversationTile`.
- `chat_thread_screen.dart`: `ChatThreadScreen` with these private widgets:
  - `_ThreadView`
  - `_Bubble`
  - `_ReplyQuote`
  - `_Composer`
- `chat_members_screen.dart`: `ChatMembersScreen`.

## Changing it safely

- The message list is a reversed `ListView` (index 0 is the newest message, at the bottom).
  Older pages load when the user scrolls near `maxScrollExtent`.
- Use `Routes.*` for every navigation.
- Reports go through `showReportSheet` (`ReportTarget.message` / `ReportTarget.conversation`).
- Show errors with `showErrorSnack` / `errorMessage`, never `toString()`.

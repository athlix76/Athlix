# features/chat/data

Wire models and REST calls for chat.

## Contents

- `chat_models.dart`: `Conversation`, `Message`, `ReplyPreview`, `ConversationMember`, plus wire
  constants (`ConversationType`, `MemberRole`, `MessageKind`, `MessageStatus`).
- `chat_api.dart`: `ChatApi` (one method per endpoint) and `chatApiProvider`. Other features use
  `eventGroup(eventId)` and `direct(userId)` to open a conversation.

## Changing it safely

- Field names match docs/api.md exactly. Add a field to `fromJson` and `toJson` together.
- An optimistic message has `id == 0` until the server returns it. Code matches it by
  `clientMsgId`, so do not use `id` as a key for unsent messages.
- Do not add logging here. Request and response bodies can contain message text.

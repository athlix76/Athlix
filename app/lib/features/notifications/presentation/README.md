# notifications/presentation

- `notifications_screen.dart`: the list. Tap = mark read, then open `routeForNotificationData`.

Patch rows through the `CursorListController`; do not reload the whole list after one tap.

# notifications/application

- `push_service.dart`: `PushService` (`start()` after sign-in, `stop()` before logout) and
  `pushServiceProvider`. Re-exports `routeForNotificationData`.
- `notification_routing.dart`: pure `routeForNotificationData(data)` → `Routes.*` or null.
- `notifications_controller.dart`: unread badge provider and mark-read actions.

Safe changes: keep every Firebase call inside try/catch, keep the token in memory only, and
call `stop()` while the session is still valid (it needs auth for the DELETE).

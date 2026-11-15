# notifications/data

- `notification_models.dart`: `AppNotification` (the API `Notification`; renamed so it does not
  clash with Flutter's `Notification`) and `DevicePlatform` values.
- `notifications_api.dart`: list, mark read, register / unregister a push device.

Field names match docs/api.md "Notifications" and "Devices". Add a field in `fromJson` first.
Never add logging here: the device calls carry the FCM token.

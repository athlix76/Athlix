# features/notifications

In-app notifications and FCM push.

## Screens

| Screen | Route | Notes |
|---|---|---|
| `NotificationsScreen` | `/notifications` (bottom tab "Alerts"; a tapped push with no specific target switches to it) | cursor list, unread count from `meta.unreadCount`, mark one read on tap, mark all read |

## Endpoints

- `GET /notifications`, `POST /notifications/read` (`{ids}` or `{all: true}`)
- `GET /notifications/unread-count` (`unreadNotificationsProvider`, the badge on the Alerts tab)
- `POST /notifications/devices` (`{platform, token, appVersion}`), `DELETE /notifications/devices` (`{token}`)

## Rules

- The FCM token is kept in memory only and is **never logged**, printed or stored.
- Every Firebase call is wrapped in try/catch: without Firebase config the app runs, push is off.
- Deep links come only from ids in `data` (+ `type`), mapped by `routeForNotificationData`.

## Changing it safely

New notification type with a new screen: add a branch to `application/notification_routing.dart`
and a unit test for it. Keep the function pure (no Flutter, no Firebase).

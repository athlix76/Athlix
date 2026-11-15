# follow/application

- `follow_controller.dart`: `FollowController` (`setFollowing`, `block`, `unblock`) and
  `followChangesProvider`, a counter other features watch to refresh after a change.

Changing it safely: call the API through this controller, not directly, so listeners refresh.

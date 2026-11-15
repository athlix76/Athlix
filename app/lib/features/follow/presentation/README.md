# follow/presentation

- `follow_button.dart`: `FollowButton`. It keeps local state after a tap; give it a key that
  includes the server flag if the parent reloads.
- `block_actions.dart`: `confirmBlock`, `confirmUnblock`.
- `follow_list_screen.dart`: `FollowListScreen`.
- `blocked_users_screen.dart`: `BlockedUsersScreen`.

Changing it safely: rows in the blocked list do not open the profile (it would be 404).

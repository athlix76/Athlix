# features/follow

Follow, unfollow, block and unblock, plus follower / following / blocked lists.

## Screens and widgets

| Widget | Route | Notes |
|---|---|---|
| `FollowListScreen(userId:, following:)` | `/profile/:id/followers`, `/profile/:id/following` | cursor lists of `UserTile` |
| `BlockedUsersScreen` | `/settings/blocked` | unblock removes the row |
| `FollowButton(userId:, initialFollowing:)` | any | toggles and reports the new `FollowState` |
| `confirmBlock` / `confirmUnblock` | any | confirm dialog, call, snack |

## Endpoints

- `POST /users/:id/follow`, `DELETE /users/:id/follow`
- `GET /users/:id/followers`, `GET /users/:id/following`
- `POST /users/:id/block`, `DELETE /users/:id/block`, `GET /me/blocks`

## Changing it safely

Every change goes through `FollowController`, which bumps `followChangesProvider`; the profile
providers watch it to reload counts and flags. Keep that bump if you add an action.
Blocking also removes follows both ways on the server; a blocked profile then returns 404.

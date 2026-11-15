# follow
Follow and block between users. A follow keeps the profile follower / following counters in step
and publishes `follow.created`; a block removes any follow in either direction and hides the two
users from each other (every route then answers 404).

## Routes
See docs/api.md "Follow and block".

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/users/:userId/follow` | Verified | target active, onboarded, not blocked either way |
| DELETE | `/users/:userId/follow` | Verified | target not blocked either way |
| GET | `/users/:userId/followers` | Verified | target's profile visibility |
| GET | `/users/:userId/following` | Verified | target's profile visibility |
| POST | `/users/:userId/block` | Verified | |
| DELETE | `/users/:userId/block` | Verified | |
| GET | `/me/blocks` | Verified | own list only |

## Tables
- Owned: `follows`, `blocks`.
- Writes through the profile helpers: `profiles.followers_count`, `profiles.following_count`.
- Read: `users`, `user_roles` (through `d.Users`), `profiles.visibility` (via `profile.ListsVisible`).

## Events
- Published: `follow.created` (`events.FollowPayload{FollowerID, FolloweeID}`) after the commit,
  only when a new follow row was inserted.
- Consumed: `user.deleted`: locks the user's follows, decrements the other side's counters,
  then deletes every follow and block that involves the user. Idempotent (a second delivery finds
  nothing); a malformed payload is dropped; DB errors are returned so the event is retried.

## Jobs
None.

## Config keys
None. Page size comes from `limit` (default 20, max 100 via `httpx.ParsePage`).

## Rules
- The caller is always `authn.UserID(c)`. Acting on yourself → 422 `userId`.
- Unknown, deleted, or (for follow and the lists) suspended or not onboarded target → 404.
  Blocked in either direction → 404, so a block cannot be detected by probing.
- Follow inserts with `INSERT IGNORE ... WHERE NOT EXISTS (blocks)`, so a block that lands
  between the check and the insert still wins (404). Following twice returns the same state,
  with no counter change and no second event.
- Unfollow and block only decrement counters for rows they actually deleted, and the decrement
  never goes below 0. Unblock always returns 204.
- Another user's lists follow their profile `visibility`: `public` shows, `followers` needs the
  caller to follow them, `private` or no profile → empty page. Your own lists are always shown.
- Lists use keyset paging on `(created_at, user id)` with an opaque cursor; a bad cursor → 422.
  Users who are not active (followers / following) or deleted (blocks) are left out, and minors'
  cities are hidden on cards (`profile.HideMinorCities`).
- Every query uses bind parameters; logs carry user ids and counts only.

## Tests
`follow_test.go`: the follow happy path (SQL order, counters, event payload) and repeat without a
second event; the block race; self → 422 on every write; the 404 table (unknown, deleted,
suspended, not onboarded, blocked either way) across follow and both lists; bad ids; unfollow with
and without a removal; rollback with no event on a DB error; block removing follows both ways and
its idempotent repeat; unblock; own followers paging (limit + 1, next cursor, row order kept,
minor city hidden, cursor args); bad cursors; other users' lists by visibility; the blocks list
skipping vanished users; and the idempotent `user.deleted` cleanup.

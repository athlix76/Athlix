# feed

Posts, likes and comments. Serves the `following` feed (people I follow plus my own posts)
and the `discover` feed (recent public posts, my sports first), a user's post list, and the
like and comment actions on a post. Ads are not read here; the app inserts them.

## Routes

All routes are on the `Verified` group. Contract: [docs/api.md#feed-module-feed](../../../../docs/api.md).

| Method | Path | Group | Permission |
|---|---|---|---|
| GET | `/feed?mode=following\|discover&cursor&limit` | Verified | any verified user |
| GET | `/users/:userId/posts` | Verified | any verified user; 404 if the user is not active or a block exists either way |
| POST | `/posts` | Verified | any verified user |
| GET | `/posts/:id` | Verified | post visible to the caller, else 404 |
| DELETE | `/posts/:id` | Verified | post author only, else 404 |
| POST | `/posts/:id/like` | Verified | post visible to the caller, else 404 |
| DELETE | `/posts/:id/like` | Verified | post visible to the caller, else 404 |
| GET | `/posts/:id/comments` | Verified | post visible to the caller, else 404 |
| POST | `/posts/:id/comments` | Verified | post visible to the caller, else 404 |
| DELETE | `/comments/:id` | Verified | comment author or post author, else 404 |

`POST /posts` and `POST /posts/:id/comments` return 200, following docs/api.md.

## Tables

- Owned: `posts`, `post_likes`, `post_comments`.
- Read only: `users`, `blocks`, `follows`, `profiles`, `profile_sports`, `sports`, `events`.
- Through platform services: `media_uploads` (`d.Media.Claim`, `d.Media.ViewURLs`) and user
  summaries (`d.Users.Summaries`, `d.Users.BlockedEitherWay`).

## Events

- Published after commit, with payload `events.PostPayload`:
  - `post.liked`: only when a new like row was inserted and the liker is not the author.
  - `post.commented`: only when the commenter is not the post author (includes `commentId`).
- Consumed: none.

## Jobs

None.

## Config keys

| Key | Default | Use |
|---|---|---|
| `feed.post_max_length` | `2000` | Maximum characters (runes) in a post body after trimming |
| `feed.post_max_media` | `4` | Maximum images per post |
| `feed.page_size` | `20` | Page size for feeds, user posts and comments; `?limit` can only lower it (hard maximum 100) |
| `feed.comment_max_length` | `1000` | Maximum characters (runes) in a comment after trimming; clamped to 1..1000 (`post_comments.body` is VARCHAR(1000)) |
| `minor.profile_default_visibility` | `"followers"` | When not `"public"`, a minor's posts default to `followers` and `public` is rejected |

## Rules

- The caller always comes from the token (`authn.UserID` / `authn.User`), never from the request.
- A post is visible to viewer V only when its status is `visible`, its author is `active`,
  there is no block in either direction, and it is public, V's own, or V follows the author.
  Every read of a post applies this rule in the SQL itself. Anything else returns 404 with the
  same response as a missing post. `hidden` posts are invisible everywhere.
- Images are signed with `d.Media.ViewURLs` only after the visibility check, i.e. only for
  rows returned by a visibility-scoped query. Authors, sports and `likedByMe` are each loaded
  in one batch per page.
- Ownership is enforced in the write query: `DELETE /posts/:id` updates
  `WHERE id = ? AND author_id = ?`, and `DELETE /comments/:id` selects
  `WHERE c.id = ? AND (c.author_id = ? OR p.author_id = ?)`. No match returns 404.
- Deletes are soft (`status = 'deleted'`).
- Likes are idempotent: `INSERT IGNORE` / `DELETE`, and `like_count` changes in the same
  transaction only if a row actually changed. Counter decrements never go below zero.
- `POST /posts`: body is optional but a post needs a body or at least one image. Media ids
  must be positive and unique, and each is claimed with purpose `post_image` inside the
  insert transaction; an unclaimable id returns 422 on `mediaUploadIds`. `sportId` must be an
  active sport and `eventId` a `published` or `completed` event (422 otherwise).
- `discover` pages in two phases carried in the cursor: `s` (posts in the viewer's sports from
  `profile_sports` and `profiles.sport_id`, newest first), then `o` (every other public post,
  newest first). A viewer with no sports goes straight to `o`.
- Comments are listed oldest first by id, and comments by users blocked either way are hidden.
- Logs carry ids only, never body text or PII. All SQL uses bind parameters.

## Tests

`feed_test.go` (sqlmock through `testutil.Deps`, handlers through `testutil.UserEngine`):

- Happy paths: get post (images signed, sport with code, likedByMe), create post (text, and
  images claimed), delete own post, like, unlike, list comments with cursor, add comment,
  delete comment, following feed, discover feed across both phases and pages, user posts.
- 404s: a followers-only post of a non-followed user, a blocked author, an inactive author,
  deleting another user's post, deleting someone else's comment, liking an invisible post,
  posts of a blocked or inactive user.
- 422s whose outcome depends on config: body length (`feed.post_max_length` 5 rejects, 10
  accepts), comment length (`feed.comment_max_length` 5 rejects 6 chars; 5000 is clamped to
  1000) and media count (`feed.post_max_media` 1 rejects, 2 accepts). Other 422s: empty
  post, duplicate or non-positive media id, bad visibility, bad sport or event id, inactive
  sport, unpublished event, unclaimable media, empty or over-long comment, bad feed mode.
- Minor visibility: defaults to `followers`, `public` rejected, allowed when the setting is
  `"public"`; adults default to `public`.
- `?limit` can only lower `feed.page_size`.
- Like idempotency: a second like neither increments the counter nor publishes; liking one's
  own post does not publish.
- `post.liked` and `post.commented` are published only after the transaction committed
  (checked from a handler registered on `d.Dispatcher`); commenting on one's own post does not publish.

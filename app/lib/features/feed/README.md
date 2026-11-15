# features/feed

Posts, likes and comments.

## Screens

| Screen | Route | Notes |
|---|---|---|
| `FeedScreen` | `/feed` (not linked from navigation: removed from the bottom tabs; route kept) | Following / Discover tabs, `home_feed` ad slot every `ads.feed_every_n_posts` posts (or once at the top), notification bell, compose button |
| `FeedList` | (widget) | one feed mode as a cursor list of `PostCard`s with the `home_feed` ad rule and an optional scrolling `header`; used by the Home tab and `FeedScreen` |
| `UserPostsScreen` | `/profile/:id/posts` | one user's posts |
| `PostComposerScreen` | `/posts/new?eventId=` | text up to `feed.post_max_length`, up to `feed.post_max_media` images, sport, visibility; pops with the new `Post` |
| `PostDetailScreen` | `/posts/:id` | post, comments (oldest first), reply box, delete own post / comment, report |

## Endpoints

- `GET /feed?mode&cursor`, `GET /users/:userId/posts`, `GET /posts/:id`
- `POST /posts`, `DELETE /posts/:id`
- `POST /posts/:id/like`, `DELETE /posts/:id/like`
- `GET /posts/:id/comments`, `POST /posts/:id/comments`, `DELETE /comments/:id`
- Media: `POST /media/uploads` + `POST /media/uploads/:id/confirm` (purpose `post_image`, via core/media)

## Rules

- Limits come from public config; the server checks again. Nothing is hard-coded.
- Image URLs are signed and short-lived; never cache them across sessions.
- Reports always go through `showReportSheet` (features/reports).
- Minors default to `followers` visibility when `minor.profile_default_visibility` is not `public`.

## Changing it safely

New post field: `Post.fromJson` → `copyWith` if it changes in place → `PostCard`. New write:
add it to `FeedController` and invalidate `postProvider(id)`.

# feed/data

- `post_models.dart`: `Post`, `Comment`, `LikeResult`, `PostInput`, `FeedMode`, `PostVisibility`.
- `feed_api.dart`: every feed endpoint, one method each.

Field names match docs/api.md "Feed". The app sends `mediaUploadIds`, never storage keys.

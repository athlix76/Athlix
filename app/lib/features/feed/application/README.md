# feed/application

- `feed_controller.dart`:
  - `postProvider(id)`
  - `FeedController`:
    - like / unlike
    - create, which uploads `post_image` files first
    - delete
    - add / delete comment

Lists patch their own rows with the returned values. Invalidate `postProvider(id)` after any
write to a post.

# feed/presentation

- `feed_screen.dart`, `user_posts_screen.dart`, `post_composer_screen.dart`,
  `post_detail_screen.dart`: the four routed screens.
- `post_card.dart`: one post with an optimistic like, delete (own) and report (others).

Ads use `AdSlot(placement: AdPlacement.homeFeed)` from features/ads only; do not build ad UI here.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../ads/presentation/ad_slot.dart';
import '../data/feed_api.dart';
import '../data/post_models.dart';
import 'post_card.dart';

/// One feed (`GET /feed?mode=following|discover`) as a cursor list of [PostCard]s.
///
/// The `home_feed` ad slot appears every `ads.feed_every_n_posts` posts, or once at the
/// top when that is not set. [header] (for example the Home tab's quick links and
/// featured events) scrolls with the posts, above the top ad.
class FeedList extends ConsumerWidget {
  const FeedList({super.key, required this.mode, required this.controller, this.header});

  final String mode;
  final CursorListController<Post> controller;
  final Widget? header;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final api = ref.watch(feedApiProvider);
    final config = ref.watch(appConfigProvider);
    final every = config.adsEnabled ? (config.adsFeedEveryNPosts ?? 0) : 0;
    final top = header;
    final showTopAd = config.adsEnabled && every <= 0;
    final ctrl = controller;
    return CursorListView<Post>(
      controller: ctrl,
      fetch: (cursor) => api.feed(mode, cursor: cursor),
      header: top == null && !showTopAd
          ? null
          : Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (top != null) top,
                if (showTopAd) const AdSlot(placement: AdPlacement.homeFeed),
              ],
            ),
      emptyText: mode == FeedMode.following
          ? 'Follow players and coaches to see their posts here.'
          : 'No posts yet.',
      padding: const EdgeInsets.only(bottom: 88),
      itemBuilder: (context, post, index) {
        final card = PostCard(
          post: post,
          onChanged: (p) => ctrl.replaceWhere((x) => x.id == p.id, (_) => p),
          onDeleted: () => ctrl.removeWhere((x) => x.id == post.id),
        );
        if (every > 0 && (index + 1) % every == 0) {
          return Column(
            mainAxisSize: MainAxisSize.min,
            children: [card, const AdSlot(placement: AdPlacement.homeFeed)],
          );
        }
        return card;
      },
    );
  }
}

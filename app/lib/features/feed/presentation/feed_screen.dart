import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/router/routes.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../notifications/application/notifications_controller.dart';
import '../data/post_models.dart';
import 'feed_list.dart';

/// `/feed`: Following and Discover as a full screen. No longer linked from the
/// navigation (the Home tab shows the feed, see [FeedList]); the route is kept.
class FeedScreen extends ConsumerStatefulWidget {
  const FeedScreen({super.key});

  @override
  ConsumerState<FeedScreen> createState() => _FeedScreenState();
}

class _FeedScreenState extends ConsumerState<FeedScreen> {
  static const _modes = [FeedMode.following, FeedMode.discover];
  final _lists = {for (final m in _modes) m: CursorListController<Post>()};

  Future<void> _compose(BuildContext tabContext) async {
    final post = await context.push<Post>(Routes.postNew);
    if (post == null || !tabContext.mounted) return;
    final index = DefaultTabController.of(tabContext).index;
    _lists[_modes[index]]?.insertFirst(post);
  }

  @override
  Widget build(BuildContext context) {
    final unread = ref.watch(unreadNotificationsProvider).valueOrNull ?? 0;
    return DefaultTabController(
      length: _modes.length,
      child: Builder(
        builder: (tabContext) => Scaffold(
          appBar: AppBar(
            title: const Text('Feed'),
            actions: [
              IconButton(
                tooltip: 'Notifications',
                onPressed: () => context.push(Routes.notifications),
                icon: Badge(
                  isLabelVisible: unread > 0,
                  label: Text(unread > 99 ? '99+' : '$unread'),
                  child: const Icon(Icons.notifications_none),
                ),
              ),
            ],
            bottom: const TabBar(tabs: [Tab(text: 'Following'), Tab(text: 'Discover')]),
          ),
          body: TabBarView(
            children: [for (final m in _modes) FeedList(mode: m, controller: _lists[m]!)],
          ),
          floatingActionButton: FloatingActionButton(
            tooltip: 'New post',
            onPressed: () => _compose(tabContext),
            child: const Icon(Icons.edit_outlined),
          ),
        ),
      ),
    );
  }
}

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/section.dart';
import '../../chat/application/chat_socket_provider.dart';
import '../../events/application/event_providers.dart';
import '../../events/presentation/event_card.dart';
import '../../feed/data/post_models.dart';
import '../../feed/presentation/feed_list.dart';
import 'home_shell.dart';

/// Home tab: the app's feed.
///
/// App bar: greeting and the chat icon (with unread count). Below it, Discover and
/// Following tabs, each a [FeedList] (`GET /feed`) whose header holds the search entry,
/// quick links and up to [_featuredOnHome] featured events. The button adds a post.
/// Notifications are a bottom tab, see [HomeShell].
class HomeScreen extends ConsumerStatefulWidget {
  const HomeScreen({super.key});

  @override
  ConsumerState<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends ConsumerState<HomeScreen> {
  // Discover first: a new user follows nobody, so Following starts empty.
  static const _modes = [FeedMode.discover, FeedMode.following];
  final _lists = {for (final m in _modes) m: CursorListController<Post>()};

  Future<void> _compose(BuildContext tabContext) async {
    final post = await context.push<Post>(Routes.postNew);
    if (post == null || !tabContext.mounted) return;
    final index = DefaultTabController.of(tabContext).index;
    _lists[_modes[index]]?.insertFirst(post);
  }

  @override
  Widget build(BuildContext context) {
    final me = ref.watch(meProvider);
    final unreadChat = ref.watch(unreadChatProvider).valueOrNull?.messages ?? 0;
    return DefaultTabController(
      length: _modes.length,
      child: Builder(
        builder: (tabContext) => Scaffold(
          appBar: AppBar(
            title: Text(me == null ? 'Athlix' : 'Hi, ${me.displayName}'),
            actions: [
              IconButton(
                tooltip: unreadChat > 0 ? 'Chat, $unreadChat unread' : 'Chat',
                icon: CountBadge(count: unreadChat, child: const Icon(Icons.chat_bubble_outline)),
                onPressed: () => context.push(Routes.chat),
              ),
            ],
            bottom: const TabBar(tabs: [Tab(text: 'Discover'), Tab(text: 'Following')]),
          ),
          body: TabBarView(
            children: [
              for (final m in _modes) FeedList(mode: m, controller: _lists[m]!, header: const _HomeHeader()),
            ],
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

/// How many featured events Home shows above the posts (the rest: "See all events").
const _featuredOnHome = 3;

/// Scrolls with the posts: search entry, quick links, featured events.
class _HomeHeader extends ConsumerWidget {
  const _HomeHeader();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final featured = ref.watch(featuredEventsProvider);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
          child: Card(
            margin: EdgeInsets.zero,
            child: ListTile(
              leading: const Icon(Icons.search),
              title: const Text('Search people, events, communities'),
              onTap: () => context.go(Routes.search), // switches to the Search tab
            ),
          ),
        ),
        const SizedBox(height: 12),
        const _QuickLinks(),
        const SectionHeader('Featured events'),
        featured.when(
          skipLoadingOnRefresh: true,
          loading: () => const Padding(
            padding: EdgeInsets.all(24),
            child: Center(child: CircularProgressIndicator()),
          ),
          error: (e, _) => ErrorView(error: e, onRetry: () => ref.invalidate(featuredEventsProvider)),
          data: (events) => events.isEmpty
              ? Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 16),
                  child: TextButton(
                    onPressed: () => context.go(Routes.events),
                    child: const Text('No featured events right now. Browse all events'),
                  ),
                )
              : Column(
                  children: [
                    for (final e in events.take(_featuredOnHome)) EventCard(event: e),
                    if (events.length > _featuredOnHome)
                      TextButton(
                        onPressed: () => context.go(Routes.events),
                        child: const Text('See all events'),
                      ),
                  ],
                ),
        ),
        const SectionHeader('Posts'),
      ],
    );
  }
}

class _QuickLinks extends StatelessWidget {
  const _QuickLinks();

  @override
  Widget build(BuildContext context) {
    const links = [
      (Icons.confirmation_number_outlined, 'My registrations', Routes.myRegistrations),
      (Icons.groups_outlined, 'Communities', Routes.communities),
      (Icons.sports_outlined, 'Consultations', Routes.consultations),
      (Icons.workspace_premium_outlined, 'Premium', Routes.premium),
    ];
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: Wrap(
        spacing: 8,
        runSpacing: 8,
        children: [
          for (final (icon, label, route) in links)
            ActionChip(
              avatar: Icon(icon, size: 18),
              label: Text(label),
              onPressed: () => context.push(route),
            ),
        ],
      ),
    );
  }
}

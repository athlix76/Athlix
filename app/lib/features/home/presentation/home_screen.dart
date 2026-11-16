import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/router/routes.dart';
import '../../../core/widgets/athlix_logo.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/page_hero.dart';
import '../../chat/application/chat_socket_provider.dart';
import '../../feed/data/post_models.dart';
import '../../feed/presentation/feed_list.dart';
import 'home_shell.dart';

/// Home tab: the app's feed, laid out like the approved home screenshot.
///
/// Header: the Athlix mark and name on the left, the chat button (unread badge, opens
/// `/chat`) on the right. Below it a "Following | Latest" pill toggle switching two
/// [FeedList]s (`GET /feed?mode=following|discover`), and the cyan "+" button for a new
/// post. Quick links live on the Profile tab; notifications are a bottom tab ([HomeShell]).
class HomeScreen extends ConsumerStatefulWidget {
  const HomeScreen({super.key});

  @override
  ConsumerState<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends ConsumerState<HomeScreen> with SingleTickerProviderStateMixin {
  static const _modes = [FeedMode.following, FeedMode.discover];
  static const _labels = ['Following', 'Latest'];
  final _lists = {for (final m in _modes) m: CursorListController<Post>()};
  late final _tabs = TabController(length: _modes.length, vsync: this)..addListener(_onTab);

  void _onTab() {
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    _tabs.dispose();
    super.dispose();
  }

  Future<void> _compose() async {
    final post = await context.push<Post>(Routes.postNew);
    if (post == null || !mounted) return;
    _lists[_modes[_tabs.index]]?.insertFirst(post);
  }

  @override
  Widget build(BuildContext context) {
    final unreadChat = ref.watch(unreadChatProvider).valueOrNull?.messages ?? 0;
    return AnnotatedRegion<SystemUiOverlayStyle>(
      value: SystemUiOverlayStyle.light,
      child: Scaffold(
        backgroundColor: Colors.transparent,
        body: SafeArea(
          bottom: false,
          child: Column(
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 16, 20, 16),
                child: Row(
                  children: [
                    const AthlixMarkTitle(size: 40),
                    const Spacer(),
                    RoundIconButton(
                      tooltip: unreadChat > 0 ? 'Chat, $unreadChat unread' : 'Chat',
                      icon: CountBadge(count: unreadChat, child: const Icon(Icons.chat_bubble_outline, size: 24)),
                      onPressed: () => context.push(Routes.chat),
                    ),
                  ],
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
                child: PillToggle(
                  labels: _labels,
                  selected: _tabs.index,
                  onSelected: (i) => _tabs.animateTo(i),
                ),
              ),
              Expanded(
                child: TabBarView(
                  controller: _tabs,
                  children: [
                    for (final m in _modes) FeedList(mode: m, controller: _lists[m]!),
                  ],
                ),
              ),
            ],
          ),
        ),
        floatingActionButton: GlowFab(tooltip: 'New post', onPressed: _compose),
      ),
    );
  }
}

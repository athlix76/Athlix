import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/theme/app_theme.dart';
import '../../../core/ws/chat_socket.dart';
import '../../chat/application/chat_socket_provider.dart';
import '../../notifications/application/notifications_controller.dart';

/// Bottom navigation for the five main tabs. Branch order must match the
/// `StatefulShellRoute` in core/router/app_router.dart: Home, Events, Search,
/// Notifications, Me.
///
/// The Notifications tab shows the unread notification count. Chat is not a tab:
/// it opens from the icon in the Home app bar, which shows the unread message
/// count (`GET /chat/unread-count`, muted conversations excluded). The shell keeps
/// that count fresh: on new/deleted message frames (debounced), when the app
/// resumes and when the user switches tabs.
class HomeShell extends ConsumerStatefulWidget {
  const HomeShell({super.key, required this.navigationShell});

  final StatefulNavigationShell navigationShell;

  @override
  ConsumerState<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends ConsumerState<HomeShell>
    with WidgetsBindingObserver {
  StreamSubscription<ServerFrame>? _frames;
  Timer? _debounce;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _frames = ref.read(chatSocketProvider).frames.listen(_onFrame);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _frames?.cancel();
    _debounce?.cancel();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) _refreshBadge();
  }

  void _onFrame(ServerFrame f) {
    if (f.type != WsType.messageNew && f.type != WsType.messageDeleted) return;
    _debounce?.cancel();
    _debounce = Timer(const Duration(seconds: 1), _refreshBadge);
  }

  void _refreshBadge() {
    if (mounted) ref.invalidate(unreadChatProvider);
  }

  void _select(int index) {
    if (index != widget.navigationShell.currentIndex) _refreshBadge();
    widget.navigationShell.goBranch(
      index,
      // Tapping the current tab again returns it to its first screen.
      initialLocation: index == widget.navigationShell.currentIndex,
    );
  }

  @override
  Widget build(BuildContext context) {
    // A new socket (user changed) needs a new subscription.
    ref.listen<ChatSocket>(chatSocketProvider, (_, next) {
      _frames?.cancel();
      _frames = next.frames.listen(_onFrame);
      _refreshBadge();
    });
    final unread = ref.watch(unreadNotificationsProvider).valueOrNull ?? 0;

    return ColoredBox(
      color: AppTheme.navy,
      child: DecoratedBox(
        decoration: const BoxDecoration(gradient: AppTheme.glowTop),
        child: DecoratedBox(
          decoration: const BoxDecoration(gradient: AppTheme.glowBottom),
          child: Scaffold(
            backgroundColor: Colors.transparent,
            body: widget.navigationShell,
            bottomNavigationBar: _BottomNav(
              index: widget.navigationShell.currentIndex,
              onSelect: _select,
              unread: unread,
            ),
          ),
        ),
      ),
    );
  }
}

/// Floating pill bottom bar: icons only, the active tab on a cyan gradient pill with its
/// label in cyan under it, and a soft cyan glow under the bar.
class _BottomNav extends StatelessWidget {
  const _BottomNav(
      {required this.index, required this.onSelect, required this.unread});

  final int index;
  final ValueChanged<int> onSelect;
  final int unread;

  @override
  Widget build(BuildContext context) {
    final items = [
      (Icons.home_outlined, 'Home'),
      (Icons.calendar_month_outlined, 'Events'),
      (Icons.search, 'Search'),
      (Icons.notifications_none, 'Notifications'),
      (Icons.person_outline, 'Profile'),
    ];
    return SafeArea(
      top: false,
      minimum: const EdgeInsets.only(bottom: 8),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 4, 16, 4),
        child: Container(
          height: 76,
          decoration: BoxDecoration(
            color: AppTheme.surface,
            borderRadius: BorderRadius.circular(38),
            border: Border.all(color: AppTheme.softBlueLine),
            boxShadow: const [
              BoxShadow(
                  color: Color(0x4005CDE0),
                  blurRadius: 28,
                  spreadRadius: -6,
                  offset: Offset(0, 6)),
            ],
          ),
          padding: const EdgeInsets.symmetric(horizontal: 6),
          child: Row(
            children: [
              for (var i = 0; i < items.length; i++)
                Expanded(
                  child: _NavItem(
                    icon: items[i].$1,
                    label: items[i].$2,
                    selected: i == index,
                    badge: i == 3 ? unread : 0,
                    onTap: () => onSelect(i),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

class _NavItem extends StatelessWidget {
  const _NavItem({
    required this.icon,
    required this.label,
    required this.selected,
    required this.badge,
    required this.onTap,
  });

  final IconData icon;
  final String label;
  final bool selected;
  final int badge;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      selected: selected,
      button: true,
      label: badge > 0 ? '$label, $badge unread' : label,
      excludeSemantics: true,
      child: InkResponse(
        onTap: onTap,
        radius: 34,
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            AnimatedContainer(
              duration: const Duration(milliseconds: 200),
              width: selected ? 64 : 44,
              height: selected ? 34 : 40,
              decoration: BoxDecoration(
                gradient: selected ? AppTheme.cyanGradient : null,
                borderRadius: BorderRadius.circular(18),
              ),
              alignment: Alignment.center,
              child: CountBadge(
                count: badge,
                child: Icon(icon,
                    size: 25, color: selected ? AppTheme.navy : AppTheme.slate),
              ),
            ),
            if (selected) ...[
              const SizedBox(height: 4),
              Text(
                label,
                maxLines: 1,
                overflow: TextOverflow.fade,
                softWrap: false,
                style: const TextStyle(
                    fontSize: 12,
                    color: AppTheme.cyan,
                    fontWeight: FontWeight.w700),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

/// Small number badge for the Notifications tab and the Home chat icon (99+ cap).
class CountBadge extends StatelessWidget {
  const CountBadge({super.key, required this.count, required this.child});

  final int count;
  final Widget child;

  @override
  Widget build(BuildContext context) => Badge(
        isLabelVisible: count > 0,
        label: Text(count > 99 ? '99+' : '$count'),
        child: child,
      );
}

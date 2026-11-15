import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

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

class _HomeShellState extends ConsumerState<HomeShell> with WidgetsBindingObserver {
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

    return Scaffold(
      body: widget.navigationShell,
      bottomNavigationBar: NavigationBar(
        selectedIndex: widget.navigationShell.currentIndex,
        onDestinationSelected: _select,
        destinations: [
          const NavigationDestination(icon: Icon(Icons.home_outlined), selectedIcon: Icon(Icons.home), label: 'Home'),
          const NavigationDestination(icon: Icon(Icons.event_outlined), selectedIcon: Icon(Icons.event), label: 'Events'),
          const NavigationDestination(icon: Icon(Icons.search_outlined), selectedIcon: Icon(Icons.search), label: 'Search'),
          NavigationDestination(
            icon: CountBadge(count: unread, child: const Icon(Icons.notifications_outlined)),
            selectedIcon: CountBadge(count: unread, child: const Icon(Icons.notifications)),
            label: 'Alerts',
            tooltip: unread > 0 ? 'Notifications, $unread unread' : 'Notifications',
          ),
          const NavigationDestination(icon: Icon(Icons.person_outline), selectedIcon: Icon(Icons.person), label: 'Me'),
        ],
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

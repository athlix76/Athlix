import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/error_view.dart';
import '../application/notification_routing.dart';
import '../application/notifications_controller.dart';
import '../data/notification_models.dart';
import '../data/notifications_api.dart';

/// `/notifications`: in-app notifications, newest first. Tapping one marks it
/// read and opens its deep link ([routeForNotificationData]).
class NotificationsScreen extends ConsumerStatefulWidget {
  const NotificationsScreen({super.key});

  @override
  ConsumerState<NotificationsScreen> createState() => _NotificationsScreenState();
}

class _NotificationsScreenState extends ConsumerState<NotificationsScreen> {
  final _list = CursorListController<AppNotification>();
  int _unread = 0;

  Future<void> _markAll() async {
    try {
      await ref.read(notificationsControllerProvider).markAllRead();
      final now = DateTime.now().toUtc();
      _list.replaceWhere((n) => !n.isRead, (n) => n.markedRead(now));
      setState(() => _unread = 0);
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  Future<void> _open(AppNotification n) async {
    if (!n.isRead) {
      try {
        await ref.read(notificationsControllerProvider).markRead(n.id);
        final now = DateTime.now().toUtc();
        _list.replaceWhere((x) => x.id == n.id, (x) => x.markedRead(now));
        if (mounted) setState(() => _unread = _unread > 0 ? _unread - 1 : 0);
      } catch (_) {
        // Opening the target matters more than the read flag.
      }
    }
    final route = routeForNotificationData(n.routingData);
    if (route != null && mounted) context.push(route);
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(notificationsApiProvider);
    return Scaffold(
      appBar: AppBar(
        title: Text(_unread > 0 ? 'Notifications ($_unread)' : 'Notifications'),
        actions: [
          TextButton(
            onPressed: _unread > 0 ? _markAll : null,
            child: const Text('Mark all read'),
          ),
        ],
      ),
      body: CursorListView<AppNotification>(
        controller: _list,
        fetch: (cursor) => api.list(cursor: cursor),
        onPage: (page) {
          final count = page.unreadCount;
          if (count != null && count != _unread) setState(() => _unread = count);
        },
        emptyText: 'No notifications yet.',
        separated: true,
        itemBuilder: (context, n, _) => _NotificationTile(n: n, onTap: () => _open(n)),
      ),
    );
  }
}

class _NotificationTile extends StatelessWidget {
  const _NotificationTile({required this.n, required this.onTap});

  final AppNotification n;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return ListTile(
      tileColor: n.isRead ? null : scheme.surfaceContainerHighest,
      leading: Icon(_icon(n.type), color: n.isRead ? scheme.outline : scheme.primary),
      title: Text(
        n.title,
        style: n.isRead ? null : const TextStyle(fontWeight: FontWeight.w600),
      ),
      subtitle: n.body.isEmpty ? null : Text(n.body, maxLines: 2, overflow: TextOverflow.ellipsis),
      trailing: Text(timeAgo(n.createdAt), style: Theme.of(context).textTheme.bodySmall),
      onTap: onTap,
    );
  }

  static IconData _icon(String type) {
    if (type.startsWith('message.')) return Icons.chat_bubble_outline;
    if (type.startsWith('post.')) return Icons.dynamic_feed_outlined;
    if (type.startsWith('follow.')) return Icons.person_add_alt;
    if (type.startsWith('community.')) return Icons.groups_outlined;
    if (type.startsWith('consultation.')) return Icons.sports;
    if (type.startsWith('registration.')) return Icons.how_to_reg_outlined;
    if (type.startsWith('location_identity.')) return Icons.place_outlined;
    if (type.startsWith('subscription.')) return Icons.workspace_premium_outlined;
    if (type.startsWith('event.') || type.startsWith('listing.')) return Icons.event_outlined;
    return Icons.notifications_none;
  }
}

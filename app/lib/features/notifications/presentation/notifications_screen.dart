import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/page_hero.dart';
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
      backgroundColor: Colors.transparent,
      body: Column(
        children: [
          PageHero(
            eyebrow: 'Notifications',
            title: 'Your corner of sport.',
            subtitle: _unread > 0
                ? '$_unread new ${_unread == 1 ? 'update' : 'updates'} from your community.'
                : 'You are all caught up.',
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 14, 8, 2),
            child: Row(
              children: [
                Expanded(child: Text('Recent activity', style: Theme.of(context).textTheme.titleMedium)),
                TextButton.icon(
                  onPressed: _unread > 0 ? _markAll : null,
                  icon: const Icon(Icons.done, size: 18),
                  label: const Text('Mark all read'),
                ),
              ],
            ),
          ),
          Expanded(
            child: CursorListView<AppNotification>(
              controller: _list,
              fetch: (cursor) => api.list(cursor: cursor),
              onPage: (page) {
                final count = page.unreadCount;
                if (count != null && count != _unread) setState(() => _unread = count);
              },
              emptyText: 'No notifications yet. Your next little win is worth celebrating.',
              padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
              itemBuilder: (context, n, _) => _NotificationTile(n: n, onTap: () => _open(n)),
            ),
          ),
        ],
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
    final theme = Theme.of(context);
    final unread = !n.isRead;
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Material(
        color: unread ? const Color(0xFF0C2030) : AppTheme.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(16),
          side: const BorderSide(color: AppTheme.line),
        ),
        clipBehavior: Clip.antiAlias,
        child: InkWell(
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(14, 14, 14, 14),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                CircleAvatar(
                  radius: 21,
                  backgroundColor: AppTheme.avatarBg,
                  child: Icon(_icon(n.type), size: 20, color: AppTheme.avatarFg),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        n.title,
                        style: theme.textTheme.bodyMedium?.copyWith(fontWeight: unread ? FontWeight.w800 : FontWeight.w600),
                      ),
                      if (n.body.isNotEmpty) ...[
                        const SizedBox(height: 2),
                        Text(n.body, maxLines: 2, overflow: TextOverflow.ellipsis, style: theme.textTheme.bodyMedium),
                      ],
                      const SizedBox(height: 4),
                      Text(timeAgo(n.createdAt), style: theme.textTheme.bodySmall),
                    ],
                  ),
                ),
                if (unread)
                  Padding(
                    padding: const EdgeInsets.only(left: 8, top: 16),
                    child: Container(
                      width: 8,
                      height: 8,
                      decoration: const BoxDecoration(color: AppTheme.cyan, shape: BoxShape.circle),
                    ),
                  ),
              ],
            ),
          ),
        ),
      ),
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

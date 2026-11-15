import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../data/notifications_api.dart';

/// Unread badge count (`GET /notifications/unread-count`). Invalidate after
/// marking notifications read.
final unreadNotificationsProvider = FutureProvider.autoDispose<int>(
  (ref) => ref.watch(notificationsApiProvider).unreadCount(),
);

/// Writes for the notifications list. Each write refreshes the badge.
class NotificationsController {
  NotificationsController(this._ref);

  final Ref _ref;

  NotificationsApi get _api => _ref.read(notificationsApiProvider);

  Future<void> markRead(int id) async {
    await _api.markRead([id]);
    _ref.invalidate(unreadNotificationsProvider);
  }

  Future<void> markAllRead() async {
    await _api.markAllRead();
    _ref.invalidate(unreadNotificationsProvider);
  }
}

final notificationsControllerProvider =
    Provider<NotificationsController>(NotificationsController.new);

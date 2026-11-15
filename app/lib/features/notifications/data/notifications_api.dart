import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/envelope.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import 'notification_models.dart';

class NotificationsApi {
  const NotificationsApi(this._api);

  final ApiClient _api;

  /// `GET /notifications` → Page with `meta.unreadCount`.
  Future<Paged<AppNotification>> list({String? cursor}) =>
      _api.page('/notifications', AppNotification.fromJson, query: {'cursor': cursor});

  /// `GET /notifications/unread-count` → the bell badge without loading a page.
  Future<int> unreadCount() =>
      _api.get('/notifications/unread-count', (d) => asInt(asMap(d)['unreadCount']));

  /// `POST /notifications/read` `{ids}`.
  Future<void> markRead(List<int> ids) =>
      _api.postVoid('/notifications/read', body: {'ids': ids});

  /// `POST /notifications/read` `{all: true}`.
  Future<void> markAllRead() => _api.postVoid('/notifications/read', body: {'all': true});

  /// `POST /notifications/devices`. The token is sent in the body only and is
  /// never logged by [ApiClient].
  Future<void> registerDevice({
    required String platform,
    required String token,
    String? appVersion,
  }) =>
      _api.postVoid('/notifications/devices', body: {
        'platform': platform,
        'token': token,
        if (appVersion != null) 'appVersion': appVersion,
      });

  /// `DELETE /notifications/devices` `{token}`.
  Future<void> unregisterDevice(String token) =>
      _api.delete('/notifications/devices', body: {'token': token});
}

final notificationsApiProvider =
    Provider<NotificationsApi>((ref) => NotificationsApi(ref.watch(apiClientProvider)));

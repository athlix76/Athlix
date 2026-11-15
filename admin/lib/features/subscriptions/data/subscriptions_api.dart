import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';
import '../../../core/models/common.dart';

const subscriptionStatuses = ['active', 'grace', 'expired', 'cancelled', 'refunded'];

class AdminSubscription {
  const AdminSubscription({
    required this.id,
    required this.planName,
    required this.planCode,
    required this.store,
    required this.status,
    required this.autoRenew,
    this.startedAt,
    this.expiresAt,
    this.user,
  });

  final int id;
  final String planName;
  final String planCode;
  final String store;
  final String status;
  final bool autoRenew;
  final DateTime? startedAt;
  final DateTime? expiresAt;
  final UserSummary? user;

  factory AdminSubscription.fromJson(Json j) {
    final plan = asJson(j['plan']);
    return AdminSubscription(
      id: asInt(j['id']),
      planName: asString(plan['name']),
      planCode: asString(plan['code']),
      store: asString(j['store']),
      status: asString(j['status']),
      autoRenew: asBool(j['autoRenew']),
      startedAt: asDate(j['startedAt']),
      expiresAt: asDate(j['expiresAt']),
      user: UserSummary.maybe(j['user']),
    );
  }
}

class SubscriptionsApi {
  SubscriptionsApi(this._client);

  final ApiClient _client;

  /// `GET /admin/subscriptions` (payments.read).
  Future<PageResult<AdminSubscription>> list({int? userId, String? status, int? planId, String? cursor}) => _client.page(
        '/admin/subscriptions',
        AdminSubscription.fromJson,
        query: {'userId': userId, 'status': status, 'planId': planId},
        cursor: cursor,
      );

  /// `POST /admin/subscriptions/grant` (plans.write). Store `manual`.
  Future<AdminSubscription> grant({required int userId, required int planId, required int days, required String reason}) async =>
      AdminSubscription.fromJson(asJson(await _client.post('/admin/subscriptions/grant', body: {
        'userId': userId,
        'planId': planId,
        'days': days,
        'reason': reason,
      })));
}

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import 'premium_models.dart';

class PremiumApi {
  const PremiumApi(this._api);

  final ApiClient _api;

  /// `[]` when `premium.enabled` is false.
  Future<List<Plan>> plans({String? audience}) =>
      _api.get('/premium/plans', (d) => asList(d, Plan.fromJson), query: {'audience': audience});

  Future<MySubscription> mySubscription() =>
      _api.get('/me/subscription', (d) => MySubscription.fromJson(asMap(d)));

  /// The server verifies with the store and never trusts the client. The
  /// purchase token is sent once, here, and never logged or stored by the app.
  Future<Subscription> verify({
    required String store,
    required String productId,
    String? purchaseToken,
    String? transactionId,
  }) =>
      _api.post(
        '/premium/purchases/verify',
        (d) => Subscription.fromJson(asMap(d)),
        body: {
          'store': store,
          'productId': productId,
          if (purchaseToken != null) 'purchaseToken': purchaseToken,
          if (transactionId != null) 'transactionId': transactionId,
        },
      );

  /// Re-links store purchases (new device / reinstall) in one call. The server
  /// verifies each one with the store; ones that belong to another account
  /// or have expired count as `notRestored`.
  Future<RestoreResult> restore(List<StorePurchase> purchases) => _api.post(
        '/premium/purchases/restore',
        (d) => RestoreResult.fromJson(asMap(d)),
        body: {'purchases': [for (final p in purchases) p.toJson()]},
      );

  /// Non-production only (`Env.showMockFlows`); the server refuses it in prod.
  Future<Subscription> mockActivate(int planId) => _api.post(
        '/premium/mock/activate',
        (d) => Subscription.fromJson(asMap(d)),
        body: {'planId': planId},
      );
}

final premiumApiProvider = Provider<PremiumApi>((ref) => PremiumApi(ref.watch(apiClientProvider)));

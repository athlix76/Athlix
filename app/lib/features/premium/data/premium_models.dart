import '../../../core/json/json_utils.dart';

/// One benefit of a plan: `{key, value, label}`. `value` is `true` or a number.
class PlanBenefit {
  const PlanBenefit({required this.key, required this.value, required this.label});

  final String key;
  final Object? value;
  final String label;

  factory PlanBenefit.fromJson(Map<String, dynamic> json) => PlanBenefit(
        key: asString(json['key']),
        value: json['value'],
        label: asString(json['label']),
      );
}

/// `Plan` (docs/api.md "Premium"). Price and benefits are DB config; never
/// hard-code them in the app.
class Plan {
  const Plan({
    required this.id,
    required this.code,
    required this.audience,
    required this.name,
    required this.description,
    required this.pricePaise,
    required this.billingPeriod,
    required this.storeProductIdAndroid,
    required this.storeProductIdIos,
    required this.benefits,
  });

  final int id;
  final String code;
  final String audience;
  final String name;
  final String? description;
  final int pricePaise;
  final String billingPeriod;
  final String? storeProductIdAndroid;
  final String? storeProductIdIos;
  final List<PlanBenefit> benefits;

  factory Plan.fromJson(Map<String, dynamic> json) => Plan(
        id: asInt(json['id']),
        code: asString(json['code']),
        audience: asString(json['audience']),
        name: asString(json['name']),
        description: asStringOrNull(json['description']),
        pricePaise: asInt(json['pricePaise']),
        billingPeriod: asString(json['billingPeriod'], 'monthly'),
        storeProductIdAndroid: asStringOrNull(json['storeProductIdAndroid']),
        storeProductIdIos: asStringOrNull(json['storeProductIdIos']),
        benefits: asList(json['benefits'], PlanBenefit.fromJson),
      );
}

/// `Subscription`.
class Subscription {
  const Subscription({
    required this.id,
    required this.plan,
    required this.store,
    required this.status,
    required this.autoRenew,
    required this.startedAt,
    required this.expiresAt,
  });

  final int id;
  final Plan plan;
  final String store;
  final String status;
  final bool autoRenew;
  final DateTime? startedAt;
  final DateTime? expiresAt;

  bool get isActive => status == 'active' || status == 'grace';

  factory Subscription.fromJson(Map<String, dynamic> json) => Subscription(
        id: asInt(json['id']),
        plan: Plan.fromJson(asMap(json['plan'])),
        store: asString(json['store']),
        status: asString(json['status']),
        autoRenew: asBool(json['autoRenew']),
        startedAt: asDateTimeOrNull(json['startedAt']),
        expiresAt: asDateTimeOrNull(json['expiresAt']),
      );

  static Subscription? maybe(Object? json) {
    final m = asMapOrNull(json);
    return m == null ? null : Subscription.fromJson(m);
  }
}

/// `GET /me/subscription` → `{active, benefits}`.
class MySubscription {
  const MySubscription({required this.active, required this.benefits});

  final Subscription? active;
  final Map<String, dynamic> benefits;

  factory MySubscription.fromJson(Map<String, dynamic> json) => MySubscription(
        active: Subscription.maybe(json['active']),
        benefits: asMapOrNull(json['benefits']) ?? const <String, dynamic>{},
      );
}

/// `store` values for `POST /premium/purchases/verify`.
abstract final class StoreName {
  static const googlePlay = 'google_play';
  static const appStore = 'app_store';
}

/// One store purchase sent to `POST /premium/purchases/verify` or `/restore`.
/// Holds a store token: never logged or stored on the device.
class StorePurchase {
  const StorePurchase({required this.store, required this.productId, this.purchaseToken, this.transactionId});

  final String store;
  final String productId;
  final String? purchaseToken;
  final String? transactionId;

  Map<String, dynamic> toJson() => {
        'store': store,
        'productId': productId,
        if (purchaseToken != null) 'purchaseToken': purchaseToken,
        if (transactionId != null) 'transactionId': transactionId,
      };

  @override
  String toString() => 'StorePurchase(<redacted>)';
}

/// `POST /premium/purchases/restore` → `{restored, notRestored, subscription}`.
class RestoreResult {
  const RestoreResult({required this.restored, required this.notRestored, required this.subscription});

  final int restored;
  final int notRestored;
  final MySubscription subscription;

  factory RestoreResult.fromJson(Map<String, dynamic> json) => RestoreResult(
        restored: asInt(json['restored']),
        notRestored: asInt(json['notRestored']),
        subscription: MySubscription.fromJson(asMapOrNull(json['subscription']) ?? const <String, dynamic>{}),
      );
}

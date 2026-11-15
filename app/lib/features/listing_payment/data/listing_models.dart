import '../../../core/json/json_utils.dart';

class RefundPolicy {
  const RefundPolicy({
    this.beforeFirstRegistrationPercent,
    this.afterRegistrationsPercent,
    this.onAdminRejectionPercent,
  });

  final int? beforeFirstRegistrationPercent;
  final int? afterRegistrationsPercent;
  final int? onAdminRejectionPercent;

  factory RefundPolicy.fromJson(Map<String, dynamic>? json) => RefundPolicy(
        beforeFirstRegistrationPercent: asIntOrNull(json?['beforeFirstRegistrationPercent']),
        afterRegistrationsPercent: asIntOrNull(json?['afterRegistrationsPercent']),
        onAdminRejectionPercent: asIntOrNull(json?['onAdminRejectionPercent']),
      );

  Map<String, dynamic> toJson() => {
        'beforeFirstRegistrationPercent': beforeFirstRegistrationPercent,
        'afterRegistrationsPercent': afterRegistrationsPercent,
        'onAdminRejectionPercent': onAdminRejectionPercent,
      };
}

/// `ListingQuote`. All amounts are integer paise; percentages are basis points.
class ListingQuote {
  const ListingQuote({
    this.feeRuleId,
    required this.ruleName,
    required this.entryFeePaise,
    required this.maxSlots,
    required this.grossEntryPaise,
    required this.percentBp,
    required this.minFeePaise,
    required this.maxFeePaise,
    required this.isFreeEvent,
    required this.baseFeePaise,
    required this.gstBp,
    required this.gstPaise,
    required this.totalPaise,
    required this.refundPolicy,
    this.refundPolicyDocumentId,
  });

  final int? feeRuleId;
  final String ruleName;
  final int entryFeePaise;
  final int maxSlots;
  final int grossEntryPaise;
  final int percentBp;
  final int minFeePaise;
  final int maxFeePaise;
  final bool isFreeEvent;
  final int baseFeePaise;
  final int gstBp;
  final int gstPaise;
  final int totalPaise;
  final RefundPolicy refundPolicy;
  final int? refundPolicyDocumentId;

  factory ListingQuote.fromJson(Map<String, dynamic> json) => ListingQuote(
        feeRuleId: asIntOrNull(json['feeRuleId']),
        ruleName: asString(json['ruleName']),
        entryFeePaise: asInt(json['entryFeePaise']),
        maxSlots: asInt(json['maxSlots']),
        grossEntryPaise: asInt(json['grossEntryPaise']),
        percentBp: asInt(json['percentBp']),
        minFeePaise: asInt(json['minFeePaise']),
        maxFeePaise: asInt(json['maxFeePaise']),
        isFreeEvent: asBool(json['isFreeEvent']),
        baseFeePaise: asInt(json['baseFeePaise']),
        gstBp: asInt(json['gstBp']),
        gstPaise: asInt(json['gstPaise']),
        totalPaise: asInt(json['totalPaise']),
        refundPolicy: RefundPolicy.fromJson(asMapOrNull(json['refundPolicy'])),
        refundPolicyDocumentId: asIntOrNull(json['refundPolicyDocumentId']),
      );

  Map<String, dynamic> toJson() => {
        'feeRuleId': feeRuleId,
        'ruleName': ruleName,
        'entryFeePaise': entryFeePaise,
        'maxSlots': maxSlots,
        'grossEntryPaise': grossEntryPaise,
        'percentBp': percentBp,
        'minFeePaise': minFeePaise,
        'maxFeePaise': maxFeePaise,
        'isFreeEvent': isFreeEvent,
        'baseFeePaise': baseFeePaise,
        'gstBp': gstBp,
        'gstPaise': gstPaise,
        'totalPaise': totalPaise,
        'refundPolicy': refundPolicy.toJson(),
        'refundPolicyDocumentId': refundPolicyDocumentId,
      };
}

abstract final class ListingOrderStatus {
  static const created = 'created';
  static const paid = 'paid';
  static const failed = 'failed';
  static const expired = 'expired';
  static const refunded = 'refunded';
  static const partiallyRefunded = 'partially_refunded';
}

abstract final class PaymentProvider {
  static const razorpay = 'razorpay';
  static const mock = 'mock';
  static const none = 'none';
}

/// `ListingOrder`. [providerKeyId] is the public checkout key id from the
/// backend; the app never holds a payment secret.
class ListingOrder {
  const ListingOrder({
    required this.orderId,
    required this.eventId,
    required this.status,
    required this.quote,
    required this.provider,
    this.providerOrderId,
    this.providerKeyId,
    required this.refundPaise,
    this.expiresAt,
    this.paidAt,
    required this.eventStatus,
  });

  final int orderId;
  final int eventId;
  final String status;
  final ListingQuote? quote;
  final String provider;
  final String? providerOrderId;
  final String? providerKeyId;
  final int refundPaise;
  final DateTime? expiresAt;
  final DateTime? paidAt;
  final String eventStatus;

  bool get isPaid => status == ListingOrderStatus.paid;

  factory ListingOrder.fromJson(Map<String, dynamic> json) {
    final q = asMapOrNull(json['quote']);
    return ListingOrder(
      orderId: asInt(json['orderId']),
      eventId: asInt(json['eventId']),
      status: asString(json['status']),
      quote: q == null ? null : ListingQuote.fromJson(q),
      provider: asString(json['provider'], PaymentProvider.none),
      providerOrderId: asStringOrNull(json['providerOrderId']),
      providerKeyId: asStringOrNull(json['providerKeyId']),
      refundPaise: asInt(json['refundPaise']),
      expiresAt: asDateTimeOrNull(json['expiresAt']),
      paidAt: asDateTimeOrNull(json['paidAt']),
      eventStatus: asString(json['eventStatus']),
    );
  }
}

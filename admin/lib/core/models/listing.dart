import '../api/json.dart';
import 'common.dart';

/// `ListingQuote` from docs/api.md (listing fee module).
class ListingQuote {
  const ListingQuote({
    required this.feeRuleId,
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
  });

  final int feeRuleId;
  final String ruleName;
  final int entryFeePaise;
  final int maxSlots;
  final int grossEntryPaise;
  final int percentBp;
  final int minFeePaise;
  final int? maxFeePaise;
  final bool isFreeEvent;
  final int baseFeePaise;
  final int gstBp;
  final int gstPaise;
  final int totalPaise;

  /// `beforeFirstRegistrationPercent`, `afterRegistrationsPercent`, `onAdminRejectionPercent`.
  final Map<String, int> refundPolicy;

  factory ListingQuote.fromJson(Json j) {
    final rp = <String, int>{};
    asJson(j['refundPolicy']).forEach((k, v) => rp[k] = asInt(v));
    return ListingQuote(
      feeRuleId: asInt(j['feeRuleId']),
      ruleName: asString(j['ruleName']),
      entryFeePaise: asInt(j['entryFeePaise']),
      maxSlots: asInt(j['maxSlots']),
      grossEntryPaise: asInt(j['grossEntryPaise']),
      percentBp: asInt(j['percentBp']),
      minFeePaise: asInt(j['minFeePaise']),
      maxFeePaise: asIntOrNull(j['maxFeePaise']),
      isFreeEvent: asBool(j['isFreeEvent']),
      baseFeePaise: asInt(j['baseFeePaise']),
      gstBp: asInt(j['gstBp']),
      gstPaise: asInt(j['gstPaise']),
      totalPaise: asInt(j['totalPaise']),
      refundPolicy: rp,
    );
  }
}

/// `ListingOrder` (+ `organiser` in the admin list).
class ListingOrder {
  const ListingOrder({
    required this.orderId,
    required this.eventId,
    required this.status,
    required this.quote,
    required this.provider,
    this.providerOrderId,
    required this.refundPaise,
    this.expiresAt,
    this.paidAt,
    this.eventStatus,
    this.organiser,
  });

  final int orderId;
  final int eventId;
  final String status;
  final ListingQuote? quote;
  final String provider;
  final String? providerOrderId;
  final int refundPaise;
  final DateTime? expiresAt;
  final DateTime? paidAt;
  final String? eventStatus;
  final UserSummary? organiser;

  int get totalPaise => quote?.totalPaise ?? 0;

  /// What can still be refunded (never below zero).
  int get refundablePaise {
    final left = totalPaise - refundPaise;
    return left < 0 ? 0 : left;
  }

  factory ListingOrder.fromJson(Json j) => ListingOrder(
        orderId: asInt(j['orderId']),
        eventId: asInt(j['eventId']),
        status: asString(j['status']),
        quote: asJsonOrNull(j['quote']) == null ? null : ListingQuote.fromJson(asJson(j['quote'])),
        provider: asString(j['provider']),
        providerOrderId: asStringOrNull(j['providerOrderId']),
        refundPaise: asInt(j['refundPaise']),
        expiresAt: asDate(j['expiresAt']),
        paidAt: asDate(j['paidAt']),
        eventStatus: asStringOrNull(j['eventStatus']),
        organiser: UserSummary.maybe(j['organiser']),
      );
}

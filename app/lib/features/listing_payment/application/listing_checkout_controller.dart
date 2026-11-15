import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/env.dart';
import '../../events/application/event_providers.dart';
import '../data/listing_api.dart';
import '../data/listing_models.dart';
import 'razorpay_checkout.dart';

final listingQuoteProvider = FutureProvider.autoDispose.family<ListingQuote, int>(
  (ref, eventId) => ref.watch(listingApiProvider).quote(eventId),
);

enum CheckoutOutcome { paid, pending, cancelled }

/// Pays the listing fee for an event:
/// order → Razorpay (or mock / free) → confirm → poll the latest order until it
/// is paid (the webhook is the backup if confirm fails).
class ListingCheckoutController {
  ListingCheckoutController(this._ref, {RazorpayCheckout? razorpay})
      : _razorpay = razorpay ?? RazorpayCheckout();

  final Ref _ref;
  final RazorpayCheckout _razorpay;

  static const pollInterval = Duration(seconds: 3);
  static const pollAttempts = 10;

  ListingApi get _api => _ref.read(listingApiProvider);

  Future<CheckoutOutcome> pay(int eventId, String eventTitle) async {
    var order = await _api.createOrder(eventId);
    if (!order.isPaid) {
      switch (order.provider) {
        case PaymentProvider.razorpay:
          final key = order.providerKeyId;
          final providerOrderId = order.providerOrderId;
          if (key == null || providerOrderId == null) {
            throw StateError('order is missing checkout details');
          }
          final r = await _razorpay.open(
            keyId: key,
            providerOrderId: providerOrderId,
            amountPaise: order.quote?.totalPaise ?? 0,
            description: 'Listing fee: $eventTitle',
          );
          switch (r) {
            case CheckoutCancelled():
              return CheckoutOutcome.cancelled;
            case CheckoutFailed(:final message):
              throw CheckoutError(message);
            case CheckoutSuccess(:final paymentId, :final signature):
              try {
                order = await _api.confirm(
                  eventId,
                  orderId: order.orderId,
                  providerPaymentId: paymentId,
                  providerSignature: signature,
                );
              } catch (_) {
                // Webhook backup: fall through to polling.
              }
          }
        case PaymentProvider.mock:
          if (!Env.showMockFlows) throw StateError('mock payments are disabled');
          order = await _api.mockPay(order.orderId);
        default:
          break;
      }
    }
    final settled = order.isPaid || await _pollOrder(eventId);
    _ref.invalidate(eventProvider(eventId));
    return settled ? CheckoutOutcome.paid : CheckoutOutcome.pending;
  }

  /// True once the latest order is paid or the event has left `draft` / `pending_payment`.
  Future<bool> _pollOrder(int eventId) async {
    for (var i = 0; i < pollAttempts; i++) {
      try {
        final o = await _api.latestOrder(eventId);
        if (o.isPaid) return true;
        if (o.eventStatus != 'pending_payment' && o.eventStatus != 'draft') return true;
      } catch (_) {
        // Keep polling; a transient error should not lose a paid order.
      }
      await Future<void>.delayed(pollInterval);
    }
    return false;
  }
}

class CheckoutError implements Exception {
  const CheckoutError(this.message);

  final String message;

  @override
  String toString() => message;
}

final listingCheckoutProvider =
    Provider<ListingCheckoutController>((ref) => ListingCheckoutController(ref));

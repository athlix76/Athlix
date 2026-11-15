import 'dart:async';

import 'package:razorpay_flutter/razorpay_flutter.dart';

/// Result of the Razorpay checkout sheet.
sealed class CheckoutResult {
  const CheckoutResult();
}

class CheckoutSuccess extends CheckoutResult {
  const CheckoutSuccess({required this.paymentId, required this.signature});

  final String paymentId;
  final String signature;

  @override
  String toString() => 'CheckoutSuccess(<redacted>)';
}

class CheckoutFailed extends CheckoutResult {
  const CheckoutFailed(this.message);

  final String message;
}

class CheckoutCancelled extends CheckoutResult {
  const CheckoutCancelled();
}

/// Opens Razorpay checkout for a backend-created order.
///
/// Only the public `providerKeyId` from `ListingOrder` is used; the app never
/// holds a key secret. No prefill: the user's mobile and email are not sent.
class RazorpayCheckout {
  Future<CheckoutResult> open({
    required String keyId,
    required String providerOrderId,
    required int amountPaise,
    required String description,
  }) {
    final rp = Razorpay();
    final done = Completer<CheckoutResult>();

    void finish(CheckoutResult r) {
      if (!done.isCompleted) done.complete(r);
      rp.clear();
    }

    rp.on(Razorpay.EVENT_PAYMENT_SUCCESS, (PaymentSuccessResponse r) {
      final id = r.paymentId;
      final sig = r.signature;
      if (id == null || sig == null) {
        finish(const CheckoutFailed('Payment response was incomplete.'));
      } else {
        finish(CheckoutSuccess(paymentId: id, signature: sig));
      }
    });
    rp.on(Razorpay.EVENT_PAYMENT_ERROR, (PaymentFailureResponse r) {
      // Code 2 = cancelled by the user (Razorpay SDK).
      if (r.code == Razorpay.PAYMENT_CANCELLED) {
        finish(const CheckoutCancelled());
      } else {
        finish(const CheckoutFailed('The payment did not go through.'));
      }
    });
    rp.on(Razorpay.EVENT_EXTERNAL_WALLET, (ExternalWalletResponse r) {
      // The wallet app completes the payment; the order is confirmed by the webhook.
      finish(const CheckoutCancelled());
    });

    try {
      rp.open({
        'key': keyId,
        'order_id': providerOrderId,
        'amount': amountPaise,
        'currency': 'INR',
        'name': 'Athlix',
        'description': description,
      });
    } catch (_) {
      finish(const CheckoutFailed('Could not open the payment screen.'));
    }
    return done.future;
  }
}

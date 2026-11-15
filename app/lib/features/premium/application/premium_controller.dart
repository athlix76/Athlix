import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:in_app_purchase/in_app_purchase.dart';

import '../../../core/session/session_controller.dart';
import '../../../core/widgets/error_view.dart';
import '../data/premium_api.dart';
import '../data/premium_models.dart';

final premiumPlansProvider = FutureProvider.autoDispose<List<Plan>>(
  (ref) => ref.watch(premiumApiProvider).plans(),
);

final mySubscriptionProvider = FutureProvider.autoDispose<MySubscription>((ref) {
  ref.watch(currentUserIdProvider);
  return ref.watch(premiumApiProvider).mySubscription();
});

bool get _isIos => !kIsWeb && defaultTargetPlatform == TargetPlatform.iOS;

/// Store product id of [plan] on this platform, or null if not sold here.
String? storeProductIdFor(Plan plan) {
  final id = _isIos ? plan.storeProductIdIos : plan.storeProductIdAndroid;
  return id == null || id.isEmpty ? null : id;
}

/// Store listings for the given product ids (prices in the user's currency
/// come from here). Empty when the store is not available (emulator, no
/// account, tests): the screen then shows the plan's own price.
final storeProductsProvider =
    FutureProvider.autoDispose.family<Map<String, ProductDetails>, String>((ref, idsKey) async {
  final ids = idsKey.split(',').where((s) => s.isNotEmpty).toSet();
  if (ids.isEmpty) return const {};
  try {
    final iap = InAppPurchase.instance;
    if (!await iap.isAvailable()) return const {};
    final res = await iap.queryProductDetails(ids);
    return {for (final p in res.productDetails) p.id: p};
  } catch (_) {
    return const {};
  }
});

enum PurchasePhase { idle, buying, verifying, done, failed }

class PurchaseState {
  const PurchaseState(this.phase, {this.message});

  final PurchasePhase phase;

  /// User-safe text for [PurchasePhase.failed] / [PurchasePhase.done].
  final String? message;

  bool get busy => phase == PurchasePhase.buying || phase == PurchasePhase.verifying;

  static const idle = PurchaseState(PurchasePhase.idle);
}

/// In-app purchase: buy in the store → `POST /premium/purchases/verify` →
/// `completePurchase` (only after the server has accepted it, so an
/// unverified purchase is redelivered by the store and retried).
///
/// The purchase token / transaction id goes to our server only; it is never
/// logged or stored on the device.
class PremiumPurchaseController extends AutoDisposeNotifier<PurchaseState> {
  StreamSubscription<List<PurchaseDetails>>? _sub;

  @override
  PurchaseState build() {
    try {
      _sub = InAppPurchase.instance.purchaseStream.listen(
        _onPurchases,
        onError: (_) => state = const PurchaseState(PurchasePhase.failed,
            message: 'The store reported an error. Please try again.'),
      );
    } catch (_) {
      _sub = null; // Store plugin not available.
    }
    ref.onDispose(() => _sub?.cancel());
    return PurchaseState.idle;
  }

  Future<void> buy(Plan plan, ProductDetails? product) async {
    if (product == null) {
      state = const PurchaseState(PurchasePhase.failed,
          message: 'This plan is not available in the store on this device.');
      return;
    }
    final me = ref.read(meProvider);
    state = const PurchaseState(PurchasePhase.buying);
    try {
      final started = await InAppPurchase.instance.buyNonConsumable(
        purchaseParam: PurchaseParam(
          productDetails: product,
          // Play obfuscatedAccountId / Apple appAccountToken: the server checks it.
          applicationUserName: me?.storeAccountToken,
        ),
      );
      if (!started) state = PurchaseState.idle;
    } catch (_) {
      state = const PurchaseState(PurchasePhase.failed,
          message: 'Could not start the purchase. Please try again.');
    }
  }

  Future<void> restore() async {
    try {
      state = const PurchaseState(PurchasePhase.verifying);
      await InAppPurchase.instance.restorePurchases(
        applicationUserName: ref.read(meProvider)?.storeAccountToken,
      );
      // Restored purchases arrive on the stream; if none come, go back to idle.
      Future<void>.delayed(const Duration(seconds: 8), () {
        if (state.phase == PurchasePhase.verifying) state = PurchaseState.idle;
      });
    } catch (_) {
      state = const PurchaseState(PurchasePhase.failed,
          message: 'Could not restore purchases. Please try again.');
    }
  }

  /// Non-production test activation (`Env.showMockFlows` only).
  Future<void> mockActivate(Plan plan) async {
    await ref.read(premiumApiProvider).mockActivate(plan.id);
    ref.invalidate(mySubscriptionProvider);
    await ref.read(sessionProvider.notifier).refreshMe();
    state = const PurchaseState(PurchasePhase.done, message: 'Test plan activated.');
  }

  Future<void> _onPurchases(List<PurchaseDetails> purchases) async {
    // Restored purchases from one store callback go to the server together.
    final restored = purchases.where((p) => p.status == PurchaseStatus.restored).toList();
    if (restored.isNotEmpty) await _restore(restored);
    for (final p in purchases) {
      switch (p.status) {
        case PurchaseStatus.pending:
          state = const PurchaseState(PurchasePhase.buying);
        case PurchaseStatus.canceled:
          await _complete(p);
          state = PurchaseState.idle;
        case PurchaseStatus.error:
          await _complete(p);
          state = const PurchaseState(PurchasePhase.failed,
              message: 'The purchase did not go through.');
        case PurchaseStatus.purchased:
          await _verify(p);
        case PurchaseStatus.restored:
          break; // handled above
      }
    }
  }

  StorePurchase _storePurchase(PurchaseDetails p) => StorePurchase(
        store: _isIos ? StoreName.appStore : StoreName.googlePlay,
        productId: p.productID,
        purchaseToken: _isIos ? null : p.verificationData.serverVerificationData,
        transactionId: _isIos ? p.purchaseID : null,
      );

  Future<void> _restore(List<PurchaseDetails> list) async {
    state = const PurchaseState(PurchasePhase.verifying);
    try {
      final res = await ref.read(premiumApiProvider).restore([for (final p in list) _storePurchase(p)]);
      for (final p in list) {
        await _complete(p);
      }
      ref.invalidate(mySubscriptionProvider);
      await ref.read(sessionProvider.notifier).refreshMe();
      state = PurchaseState(
        PurchasePhase.done,
        message: res.restored > 0
            ? (res.subscription.active != null ? 'Premium restored.' : 'Purchases restored; none is active now.')
            : 'No purchases could be restored to this account.',
      );
    } catch (e) {
      // Not completed: the store redelivers them on the next restore.
      state = PurchaseState(PurchasePhase.failed, message: errorMessage(e));
    }
  }

  Future<void> _verify(PurchaseDetails p) async {
    state = const PurchaseState(PurchasePhase.verifying);
    try {
      final sp = _storePurchase(p);
      await ref.read(premiumApiProvider).verify(
            store: sp.store,
            productId: sp.productId,
            purchaseToken: sp.purchaseToken,
            transactionId: sp.transactionId,
          );
      await _complete(p);
      ref.invalidate(mySubscriptionProvider);
      await ref.read(sessionProvider.notifier).refreshMe();
      state = const PurchaseState(PurchasePhase.done, message: 'Premium is active. Thank you!');
    } catch (e) {
      // Not completed: the store redelivers it and we verify again later.
      state = PurchaseState(PurchasePhase.failed, message: errorMessage(e));
    }
  }

  Future<void> _complete(PurchaseDetails p) async {
    if (!p.pendingCompletePurchase) return;
    try {
      await InAppPurchase.instance.completePurchase(p);
    } catch (_) {}
  }
}

final premiumPurchaseProvider =
    NotifierProvider.autoDispose<PremiumPurchaseController, PurchaseState>(
        PremiumPurchaseController.new);

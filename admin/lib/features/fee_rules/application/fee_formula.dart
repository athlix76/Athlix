import '../data/fee_rules_api.dart';

/// `round(numerator / denominator)` half-up, for non-negative integers.
int divRoundHalfUp(int numerator, int denominator) => (numerator * 2 + denominator) ~/ (denominator * 2);

class FeeBreakdown {
  const FeeBreakdown({
    required this.grossEntryPaise,
    required this.isFreeEvent,
    required this.rawPercentFeePaise,
    required this.baseFeePaise,
    required this.gstPaise,
    required this.clampedTo,
  });

  final int grossEntryPaise;
  final bool isFreeEvent;

  /// gross × percent before the min/max clamp (0 for a free event).
  final int rawPercentFeePaise;
  final int baseFeePaise;
  final int gstPaise;

  /// 'min', 'max' or null when the clamp did not apply.
  final String? clampedTo;

  int get totalPaise => baseFeePaise + gstPaise;
}

/// Display-only mirror of the backend listing fee formula (docs/api.md,
/// "Listing fee"), used to preview an unsaved rule while editing. The quote
/// an organiser pays always comes from the server.
FeeBreakdown computeListingFee(FeeRule rule, {required int entryFeePaise, required int maxSlots}) {
  final gross = entryFeePaise * maxSlots;
  if (entryFeePaise == 0) {
    final base = rule.freeEventFeePaise;
    return FeeBreakdown(
      grossEntryPaise: 0,
      isFreeEvent: true,
      rawPercentFeePaise: 0,
      baseFeePaise: base,
      gstPaise: divRoundHalfUp(base * rule.gstBp, 10000),
      clampedTo: null,
    );
  }
  final raw = divRoundHalfUp(gross * rule.percentBp, 10000);
  var base = raw;
  String? clamped;
  if (base < rule.minFeePaise) {
    base = rule.minFeePaise;
    clamped = 'min';
  }
  final max = rule.maxFeePaise;
  if (max != null && base > max) {
    base = max;
    clamped = 'max';
  }
  return FeeBreakdown(
    grossEntryPaise: gross,
    isFreeEvent: false,
    rawPercentFeePaise: raw,
    baseFeePaise: base,
    gstPaise: divRoundHalfUp(base * rule.gstBp, 10000),
    clampedTo: clamped,
  );
}

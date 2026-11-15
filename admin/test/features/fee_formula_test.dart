import 'package:athlix_admin/features/fee_rules/application/fee_formula.dart';
import 'package:athlix_admin/features/fee_rules/data/fee_rules_api.dart';
import 'package:flutter_test/flutter_test.dart';

// Test fixtures only. Real rules always come from GET /admin/fee-rules.
const _rule = FeeRule(
  name: 'fixture',
  percentBp: 500,
  minFeePaise: 9900,
  maxFeePaise: 499900,
  freeEventFeePaise: 9900,
  gstBp: 1800,
  priority: 0,
  active: true,
);

void main() {
  test('divRoundHalfUp rounds half up', () {
    expect(divRoundHalfUp(5, 10), 1);
    expect(divRoundHalfUp(4, 10), 0);
    expect(divRoundHalfUp(15, 10), 2);
    expect(divRoundHalfUp(0, 10), 0);
  });

  test('percent fee inside the clamp', () {
    final f = computeListingFee(_rule, entryFeePaise: 50000, maxSlots: 16); // gross 8,00,000 paise
    expect(f.grossEntryPaise, 800000);
    expect(f.rawPercentFeePaise, 40000);
    expect(f.baseFeePaise, 40000);
    expect(f.clampedTo, isNull);
    expect(f.gstPaise, 7200);
    expect(f.totalPaise, 47200);
  });

  test('clamps to the minimum', () {
    final f = computeListingFee(_rule, entryFeePaise: 1000, maxSlots: 10);
    expect(f.rawPercentFeePaise, 500);
    expect(f.baseFeePaise, 9900);
    expect(f.clampedTo, 'min');
  });

  test('clamps to the maximum', () {
    final f = computeListingFee(_rule, entryFeePaise: 1000000, maxSlots: 128);
    expect(f.baseFeePaise, 499900);
    expect(f.clampedTo, 'max');
  });

  test('no cap when maxFeePaise is null', () {
    const uncapped = FeeRule(
      name: 'fixture',
      percentBp: 500,
      minFeePaise: 9900,
      freeEventFeePaise: 9900,
      gstBp: 1800,
      priority: 0,
      active: true,
    );
    final f = computeListingFee(uncapped, entryFeePaise: 1000000, maxSlots: 128);
    expect(f.baseFeePaise, 6400000);
    expect(f.clampedTo, isNull);
  });

  test('free event uses the flat fee', () {
    final f = computeListingFee(_rule, entryFeePaise: 0, maxSlots: 64);
    expect(f.isFreeEvent, isTrue);
    expect(f.baseFeePaise, 9900);
    expect(f.gstPaise, 1782);
  });
}

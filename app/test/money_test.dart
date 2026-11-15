import 'package:athlix/core/money/money.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('formatPaise', () {
    test('basic and Indian grouping', () {
      expect(formatPaise(0), '₹0.00');
      expect(formatPaise(5), '₹0.05');
      expect(formatPaise(49900), '₹499.00');
      expect(formatPaise(100000), '₹1,000.00');
      expect(formatPaise(12345678), '₹1,23,456.78');
      expect(formatPaise(1000000000), '₹1,00,00,000.00');
    });

    test('compact drops .00 only for whole rupees', () {
      expect(formatPaise(49900, compact: true), '₹499');
      expect(formatPaise(49950, compact: true), '₹499.50');
    });

    test('negative amounts', () {
      expect(formatPaise(-1050), '-₹10.50');
    });
  });

  test('formatEntryFee', () {
    expect(formatEntryFee(0), 'Free');
    expect(formatEntryFee(50000), '₹500');
  });

  test('formatBp', () {
    expect(formatBp(500), '5%');
    expect(formatBp(1850), '18.5%');
    expect(formatBp(125), '1.25%');
    expect(formatBp(0), '0%');
  });

  group('parseRupeesToPaise', () {
    test('valid inputs', () {
      expect(parseRupeesToPaise('500'), 50000);
      expect(parseRupeesToPaise('499.5'), 49950);
      expect(parseRupeesToPaise('499.05'), 49905);
      expect(parseRupeesToPaise('1,200.75'), 120075);
      expect(parseRupeesToPaise(' ₹0 '), 0);
    });

    test('invalid inputs', () {
      expect(parseRupeesToPaise(''), isNull);
      expect(parseRupeesToPaise('-5'), isNull);
      expect(parseRupeesToPaise('1.234'), isNull);
      expect(parseRupeesToPaise('abc'), isNull);
      expect(parseRupeesToPaise('1.'), isNull);
    });

    test('round trips with paiseToRupeeInput', () {
      for (final p in [0, 1, 99, 100, 49950, 120075, 50000]) {
        expect(parseRupeesToPaise(paiseToRupeeInput(p)), p);
      }
    });
  });

  test('paiseToRupeeInput', () {
    expect(paiseToRupeeInput(50000), '500');
    expect(paiseToRupeeInput(49950), '499.50');
    expect(paiseToRupeeInput(5), '0.05');
  });
}

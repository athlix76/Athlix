import 'package:athlix_admin/features/config/application/config_value_codec.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  ParsedConfigValue p(String text, String type) => parseConfigValue(text, type);

  group('int', () {
    test('accepts whole numbers, trimmed', () {
      expect(p(' 30 ', ConfigValueType.integer).value, 30);
      expect(p('-5', ConfigValueType.integer).value, -5);
      expect(p('0', ConfigValueType.integer).isOk, isTrue);
    });

    test('rejects decimals, words, empty and spaces inside', () {
      for (final bad in ['2.5', 'abc', '', '1 000', '1e3', '+3']) {
        expect(p(bad, ConfigValueType.integer).isOk, isFalse, reason: bad);
      }
    });
  });

  group('decimal', () {
    test('accepts whole and fractional numbers', () {
      expect(p('2.5', ConfigValueType.decimal).value, 2.5);
      expect(p('3', ConfigValueType.decimal).value, 3.0);
      expect(p('-0.25', ConfigValueType.decimal).value, -0.25);
    });

    test('rejects malformed numbers', () {
      for (final bad in ['.5', '1.', '1,5', 'NaN', '']) {
        expect(p(bad, ConfigValueType.decimal).isOk, isFalse, reason: bad);
      }
    });
  });

  group('bool', () {
    test('only lowercase true / false', () {
      expect(p('true', ConfigValueType.boolean).value, true);
      expect(p('false', ConfigValueType.boolean).value, false);
      for (final bad in ['True', '1', 'yes', '']) {
        expect(p(bad, ConfigValueType.boolean).isOk, isFalse, reason: bad);
      }
    });
  });

  group('string', () {
    test('trims the ends, keeps inner spaces', () {
      expect(p('  hello  world ', ConfigValueType.string).value, 'hello  world');
      expect(p('', ConfigValueType.string).value, '');
    });
  });

  group('string_list', () {
    test('one item per line, blanks dropped', () {
      expect(p('aadhaar\n\n  passport \nvoter_id\n', ConfigValueType.stringList).value, ['aadhaar', 'passport', 'voter_id']);
      expect(p('', ConfigValueType.stringList).value, <String>[]);
    });

    test('rejects duplicates', () {
      final r = p('a\nb\na', ConfigValueType.stringList);
      expect(r.isOk, isFalse);
      expect(r.error, contains('"a"'));
    });

    test('round-trips through configValueToText', () {
      final text = configValueToText(['x', 'y'], ConfigValueType.stringList);
      expect(text, 'x\ny');
      expect(p(text, ConfigValueType.stringList).value, ['x', 'y']);
    });
  });

  group('json', () {
    test('accepts objects, arrays and scalars', () {
      expect(p('{"a": [1, 2]}', ConfigValueType.json).value, {
        'a': [1, 2],
      });
      expect(p('[]', ConfigValueType.json).value, <Object?>[]);
      expect(p('5', ConfigValueType.json).value, 5);
    });

    test('rejects invalid JSON and empty text', () {
      expect(p('{a: 1}', ConfigValueType.json).error, startsWith('Invalid JSON'));
      expect(p('   ', ConfigValueType.json).isOk, isFalse);
    });

    test('pretty-prints for the editor', () {
      expect(configValueToText({'a': 1}, ConfigValueType.json), '{\n  "a": 1\n}');
    });
  });

  test('unknown type fails instead of guessing', () {
    expect(p('1', 'date').isOk, isFalse);
  });

  group('configValuesEqual', () {
    test('deep, ignores map key order', () {
      expect(configValuesEqual({'a': 1, 'b': [1, 2]}, {'b': [1, 2], 'a': 1}), isTrue);
      expect(configValuesEqual([1, 2], [2, 1]), isFalse);
      expect(configValuesEqual({'a': 1}, {'a': 1, 'b': null}), isFalse);
    });

    test('numbers compare by value', () {
      expect(configValuesEqual(1, 1.0), isTrue);
      expect(configValuesEqual('1', 1), isFalse);
    });
  });
}

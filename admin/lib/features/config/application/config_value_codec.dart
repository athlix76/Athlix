import 'dart:convert';

/// Config value types from docs/config-system.md.
abstract final class ConfigValueType {
  static const integer = 'int';
  static const decimal = 'decimal';
  static const boolean = 'bool';
  static const string = 'string';
  static const stringList = 'string_list';
  static const json = 'json';

  static const values = [integer, decimal, boolean, string, stringList, json];
}

/// Result of parsing the editor text: either [value] or [error].
class ParsedConfigValue {
  const ParsedConfigValue.ok(this.value) : error = null;
  const ParsedConfigValue.fail(this.error) : value = null;

  final Object? value;
  final String? error;

  bool get isOk => error == null;
}

const _pretty = JsonEncoder.withIndent('  ');

/// Text shown in the editor (and in diffs) for a stored [value].
/// `string_list` is one item per line; `json` is pretty-printed.
String configValueToText(Object? value, String valueType) {
  switch (valueType) {
    case ConfigValueType.stringList:
      return value is List ? value.map((e) => '$e').join('\n') : '';
    case ConfigValueType.json:
      return _pretty.convert(value);
    case ConfigValueType.string:
      return value == null ? '' : '$value';
    default:
      return value == null ? '' : '$value';
  }
}

final _intRe = RegExp(r'^-?\d+$');
final _decimalRe = RegExp(r'^-?\d+(\.\d+)?$');

/// Parses and validates editor [text] for [valueType]. The backend checks
/// again (422); this only catches mistakes before the confirmation dialog.
ParsedConfigValue parseConfigValue(String text, String valueType) {
  final t = text.trim();
  switch (valueType) {
    case ConfigValueType.integer:
      if (!_intRe.hasMatch(t)) return const ParsedConfigValue.fail('Enter a whole number, for example 30');
      final v = int.tryParse(t);
      return v == null ? const ParsedConfigValue.fail('Number is too large') : ParsedConfigValue.ok(v);
    case ConfigValueType.decimal:
      if (!_decimalRe.hasMatch(t)) return const ParsedConfigValue.fail('Enter a number, for example 2.5');
      return ParsedConfigValue.ok(double.parse(t));
    case ConfigValueType.boolean:
      if (t == 'true') return const ParsedConfigValue.ok(true);
      if (t == 'false') return const ParsedConfigValue.ok(false);
      return const ParsedConfigValue.fail('Must be true or false');
    case ConfigValueType.string:
      // Strings keep inner spaces; only the surrounding whitespace is trimmed.
      return ParsedConfigValue.ok(t);
    case ConfigValueType.stringList:
      final items = text.split('\n').map((l) => l.trim()).where((l) => l.isNotEmpty).toList();
      final seen = <String>{};
      for (final i in items) {
        if (!seen.add(i)) return ParsedConfigValue.fail('Duplicate item "$i"');
      }
      return ParsedConfigValue.ok(items);
    case ConfigValueType.json:
      if (t.isEmpty) return const ParsedConfigValue.fail('Enter JSON, for example {} or []');
      try {
        return ParsedConfigValue.ok(jsonDecode(t));
      } on FormatException catch (e) {
        return ParsedConfigValue.fail('Invalid JSON: ${e.message}');
      }
    default:
      return ParsedConfigValue.fail('Unknown value type "$valueType"');
  }
}

/// True when two decoded JSON values are the same: deep, map key order does
/// not matter, and `1 == 1.0` (numbers compare by value).
bool configValuesEqual(Object? a, Object? b) {
  if (a is Map && b is Map) {
    if (a.length != b.length) return false;
    for (final k in a.keys) {
      if (!b.containsKey(k) || !configValuesEqual(a[k], b[k])) return false;
    }
    return true;
  }
  if (a is List && b is List) {
    if (a.length != b.length) return false;
    for (var i = 0; i < a.length; i++) {
      if (!configValuesEqual(a[i], b[i])) return false;
    }
    return true;
  }
  return a == b;
}

/// Money is integer **paise** everywhere (docs/api.md "Types"). These helpers
/// format and parse without ever going through a floating point number.
library;

String _groupIndian(int rupees) {
  final s = rupees.toString();
  if (s.length <= 3) return s;
  final last3 = s.substring(s.length - 3);
  var rest = s.substring(0, s.length - 3);
  final groups = <String>[];
  while (rest.length > 2) {
    groups.insert(0, rest.substring(rest.length - 2));
    rest = rest.substring(0, rest.length - 2);
  }
  if (rest.isNotEmpty) groups.insert(0, rest);
  return '${groups.join(',')},$last3';
}

/// `49900` → `₹499.00`, `12345678` → `₹1,23,456.78` (Indian digit grouping).
///
/// [compact] drops `.00` for whole rupees (`₹499`), keeping paise when present.
String formatPaise(int paise, {bool compact = false}) {
  final negative = paise < 0;
  final abs = paise.abs();
  final rupees = abs ~/ 100;
  final rem = abs % 100;
  final grouped = _groupIndian(rupees);
  final body = compact && rem == 0 ? grouped : '$grouped.${rem.toString().padLeft(2, '0')}';
  return '${negative ? '-' : ''}₹$body';
}

/// Entry fee display: `0` → "Free".
String formatEntryFee(int paise) => paise == 0 ? 'Free' : formatPaise(paise, compact: true);

/// Basis points → percent text: `500` → `5%`, `1850` → `18.5%`, `125` → `1.25%`.
String formatBp(int bp) {
  final whole = bp ~/ 100;
  final frac = (bp.abs() % 100);
  if (frac == 0) return '$whole%';
  var f = frac.toString().padLeft(2, '0');
  if (f.endsWith('0')) f = f.substring(0, 1);
  return '${bp < 0 && whole == 0 ? '-' : ''}$whole.$f%';
}

/// Parses what a user typed in a rupee field into paise.
/// `"500"` → 50000, `"499.5"` → 49950, `"1,200.75"` → 120075.
/// Returns null for empty, negative, or more than 2 decimals.
int? parseRupeesToPaise(String input) {
  final s = input.replaceAll(',', '').replaceAll('₹', '').trim();
  if (s.isEmpty) return null;
  final m = RegExp(r'^(\d+)(?:\.(\d{1,2}))?$').firstMatch(s);
  if (m == null) return null;
  final rupees = int.tryParse(m.group(1)!);
  if (rupees == null) return null;
  final fracText = (m.group(2) ?? '').padRight(2, '0');
  final paise = fracText.isEmpty ? 0 : int.parse(fracText);
  return rupees * 100 + paise;
}

/// Paise → the plain number shown in an editable rupee field (`50000` → `500`,
/// `49950` → `499.50`).
String paiseToRupeeInput(int paise) {
  final r = paise ~/ 100;
  final p = paise % 100;
  return p == 0 ? '$r' : '$r.${p.toString().padLeft(2, '0')}';
}

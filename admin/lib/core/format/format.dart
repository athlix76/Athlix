/// Display and input helpers for money (integer paise), basis points and times.
library;

String _groupIndian(String digits) {
  if (digits.length <= 3) return digits;
  final last3 = digits.substring(digits.length - 3);
  var rest = digits.substring(0, digits.length - 3);
  final parts = <String>[];
  while (rest.length > 2) {
    parts.insert(0, rest.substring(rest.length - 2));
    rest = rest.substring(0, rest.length - 2);
  }
  if (rest.isNotEmpty) parts.insert(0, rest);
  return '${parts.join(',')},$last3';
}

/// 1234567 paise → `₹12,345.67` (Indian digit grouping).
String formatPaise(int? paise) {
  if (paise == null) return '—';
  final neg = paise < 0;
  final abs = paise.abs();
  final rupees = _groupIndian((abs ~/ 100).toString());
  final p = (abs % 100).toString().padLeft(2, '0');
  return '${neg ? '-' : ''}₹$rupees.$p';
}

/// Paise → plain rupees text for an input field (`9900` → `99.00`).
String paiseToInput(int? paise) {
  if (paise == null) return '';
  final abs = paise.abs();
  return '${paise < 0 ? '-' : ''}${abs ~/ 100}.${(abs % 100).toString().padLeft(2, '0')}';
}

final _rupeesRe = RegExp(r'^\d+(\.\d{1,2})?$');

/// `99`, `99.5`, `1,234.50` → paise. Null when invalid or negative.
int? parseRupeesToPaise(String input) {
  final s = input.trim().replaceAll(',', '');
  if (!_rupeesRe.hasMatch(s)) return null;
  final parts = s.split('.');
  final whole = int.parse(parts[0]);
  final frac = parts.length > 1 ? int.parse(parts[1].padRight(2, '0')) : 0;
  return whole * 100 + frac;
}

/// 500 → `5%`, 250 → `2.5%`, 1825 → `18.25%`.
String formatBp(int? bp) {
  if (bp == null) return '—';
  final whole = bp ~/ 100;
  final frac = bp % 100;
  if (frac == 0) return '$whole%';
  var f = frac.toString().padLeft(2, '0');
  if (f.endsWith('0')) f = f.substring(0, 1);
  return '$whole.$f%';
}

/// Basis points → percent text for an input (`500` → `5`, `250` → `2.5`).
String bpToInput(int? bp) => bp == null ? '' : formatBp(bp).replaceAll('%', '');

final _percentRe = RegExp(r'^\d+(\.\d{1,2})?$');

/// `5` → 500, `2.25` → 225. Null when invalid. Upper bound 100% unless [max] given.
int? parsePercentToBp(String input, {int max = 10000}) {
  final s = input.trim().replaceAll('%', '');
  if (!_percentRe.hasMatch(s)) return null;
  final parts = s.split('.');
  final bp = int.parse(parts[0]) * 100 + (parts.length > 1 ? int.parse(parts[1].padRight(2, '0')) : 0);
  return bp > max ? null : bp;
}

String _two(int n) => n.toString().padLeft(2, '0');

/// Local time `2026-11-12 15:00`.
String formatDateTime(DateTime? t) {
  if (t == null) return '—';
  final l = t.toLocal();
  return '${l.year}-${_two(l.month)}-${_two(l.day)} ${_two(l.hour)}:${_two(l.minute)}';
}

/// Local date `2026-11-12`.
String formatDate(DateTime? t) {
  if (t == null) return '—';
  final l = t.toLocal();
  return '${l.year}-${_two(l.month)}-${_two(l.day)}';
}

/// `YYYY-MM-DD` for API date filters.
String apiDate(DateTime t) => '${t.year}-${_two(t.month)}-${_two(t.day)}';

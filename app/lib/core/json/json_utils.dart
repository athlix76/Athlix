/// Small, null-safe readers for decoded JSON (`jsonDecode` output).
///
/// The API sends optional fields as `null`; these helpers never throw on a
/// missing or wrongly typed value, they fall back instead.
library;

Map<String, dynamic> asMap(Object? v) =>
    v is Map ? v.cast<String, dynamic>() : <String, dynamic>{};

Map<String, dynamic>? asMapOrNull(Object? v) =>
    v is Map ? v.cast<String, dynamic>() : null;

List<T> asList<T>(Object? v, T Function(Map<String, dynamic> json) fromJson) {
  if (v is! List) return <T>[];
  return v
      .whereType<Map<dynamic, dynamic>>()
      .map((e) => fromJson(e.cast<String, dynamic>()))
      .toList();
}

List<String> asStringList(Object? v) =>
    v is List ? v.where((e) => e != null).map((e) => '$e').toList() : <String>[];

List<int> asIntList(Object? v) => v is List
    ? v.map(asIntOrNull).whereType<int>().toList()
    : <int>[];

int asInt(Object? v, [int fallback = 0]) => asIntOrNull(v) ?? fallback;

int? asIntOrNull(Object? v) {
  if (v is int) return v;
  if (v is num) return v.toInt();
  if (v is String) return int.tryParse(v);
  return null;
}

num? asNumOrNull(Object? v) {
  if (v is num) return v;
  if (v is String) return num.tryParse(v);
  return null;
}

String asString(Object? v, [String fallback = '']) => v == null ? fallback : '$v';

String? asStringOrNull(Object? v) => v == null ? null : '$v';

bool asBool(Object? v, [bool fallback = false]) => v is bool ? v : fallback;

bool? asBoolOrNull(Object? v) => v is bool ? v : null;

DateTime? asDateTimeOrNull(Object? v) =>
    v is String && v.isNotEmpty ? DateTime.tryParse(v)?.toUtc() : null;

DateTime asDateTime(Object? v) =>
    asDateTimeOrNull(v) ?? DateTime.fromMillisecondsSinceEpoch(0, isUtc: true);

String _two(int n) => n.toString().padLeft(2, '0');

/// RFC 3339 in UTC, `2026-11-12T09:30:00Z` (milliseconds only when non-zero),
/// the format the API uses.
String formatRfc3339(DateTime d) {
  final u = d.toUtc();
  final base = '${u.year.toString().padLeft(4, '0')}-${_two(u.month)}-${_two(u.day)}'
      'T${_two(u.hour)}:${_two(u.minute)}:${_two(u.second)}';
  if (u.millisecond == 0 && u.microsecond == 0) return '${base}Z';
  return '$base.${u.millisecond.toString().padLeft(3, '0')}Z';
}

String? formatRfc3339OrNull(DateTime? d) => d == null ? null : formatRfc3339(d);

/// `YYYY-MM-DD` for date-only fields (e.g. `achievedOn`).
String formatDateOnly(DateTime d) =>
    '${d.year.toString().padLeft(4, '0')}-${_two(d.month)}-${_two(d.day)}';

/// Drops null values and empty strings so they are not sent as query params.
Map<String, dynamic> compactQuery(Map<String, Object?> q) {
  final out = <String, dynamic>{};
  q.forEach((k, v) {
    if (v == null) return;
    if (v is String && v.isEmpty) return;
    out[k] = v is bool || v is num ? '$v' : v;
  });
  return out;
}

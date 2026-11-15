/// Small, forgiving readers for decoded JSON. Ids arrive as JSON numbers, which
/// may decode as `int` or `double` on the web, so always go through these.
library;

typedef Json = Map<String, dynamic>;

int asInt(Object? v, [int fallback = 0]) {
  if (v is num) return v.toInt();
  if (v is String) return int.tryParse(v) ?? fallback;
  return fallback;
}

int? asIntOrNull(Object? v) {
  if (v == null) return null;
  if (v is num) return v.toInt();
  if (v is String) return int.tryParse(v);
  return null;
}

String asString(Object? v, [String fallback = '']) => v == null ? fallback : '$v';

String? asStringOrNull(Object? v) => v?.toString();

bool asBool(Object? v, [bool fallback = false]) {
  if (v is bool) return v;
  if (v is num) return v != 0;
  if (v is String) return v == 'true' || v == '1';
  return fallback;
}

DateTime? asDate(Object? v) => v is String ? DateTime.tryParse(v) : null;

Json asJson(Object? v) {
  if (v is Map) return v.map((k, val) => MapEntry('$k', val));
  return <String, dynamic>{};
}

Json? asJsonOrNull(Object? v) => v is Map ? asJson(v) : null;

List<T> asList<T>(Object? v, T Function(Json) parse) {
  if (v is! List) return <T>[];
  return v.whereType<Map<dynamic, dynamic>>().map((e) => parse(asJson(e))).toList();
}

List<String> asStringList(Object? v) {
  if (v is! List) return const <String>[];
  return v.map((e) => '$e').toList();
}

/// Removes null and empty-string values so they are not sent as `?x=`.
Map<String, dynamic> cleanQuery(Map<String, Object?> q) {
  final out = <String, dynamic>{};
  q.forEach((k, v) {
    if (v == null) return;
    if (v is String && v.trim().isEmpty) return;
    out[k] = v is bool ? (v ? 'true' : 'false') : '$v';
  });
  return out;
}

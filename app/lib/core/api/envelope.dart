import '../json/json_utils.dart';
import 'api_exception.dart';

/// `meta` of a success envelope (pagination and counters).
class PageMeta {
  const PageMeta({this.nextCursor, this.unreadCount, this.total});

  final String? nextCursor;
  final int? unreadCount;
  final int? total;

  static const empty = PageMeta();

  factory PageMeta.fromJson(Map<String, dynamic>? json) {
    if (json == null) return empty;
    final cursor = asStringOrNull(json['nextCursor']);
    return PageMeta(
      nextCursor: cursor == null || cursor.isEmpty ? null : cursor,
      unreadCount: asIntOrNull(json['unreadCount']),
      total: asIntOrNull(json['total']),
    );
  }
}

/// A parsed success envelope: `{"data": ..., "meta": {...}?}`.
class Envelope {
  const Envelope(this.data, this.meta);

  final Object? data;
  final PageMeta meta;
}

/// One page of a cursor-paginated list. No [nextCursor] = last page.
class Paged<T> {
  const Paged({required this.items, this.nextCursor, this.unreadCount, this.total});

  final List<T> items;
  final String? nextCursor;
  final int? unreadCount;
  final int? total;

  bool get hasMore => nextCursor != null;

  factory Paged.fromEnvelope(Envelope env, T Function(Map<String, dynamic>) fromJson) =>
      Paged(
        items: asList(env.data, fromJson),
        nextCursor: env.meta.nextCursor,
        unreadCount: env.meta.unreadCount,
        total: env.meta.total,
      );
}

/// Parses a decoded response body.
///
/// * 2xx with `{"data": ...}` → [Envelope].
/// * 204 / empty body → `Envelope(null, PageMeta.empty)`.
/// * any body with `{"error": {...}}` → throws [ApiException] with that code.
/// * a non-2xx status without an error object → [ApiException] mapped by status.
Envelope parseEnvelope(Object? body, {int? statusCode, String? requestId}) {
  final ok = statusCode == null || (statusCode >= 200 && statusCode < 300);
  if (body is Map && body['error'] != null) {
    throw apiExceptionFromBody(body, statusCode: statusCode, requestId: requestId);
  }
  if (!ok) {
    throw apiExceptionFromBody(body, statusCode: statusCode, requestId: requestId);
  }
  if (body == null || (body is String && body.isEmpty)) {
    return const Envelope(null, PageMeta.empty);
  }
  if (body is Map) {
    final map = body.cast<String, dynamic>();
    if (!map.containsKey('data')) {
      throw ApiException.unknown(statusCode);
    }
    return Envelope(map['data'], PageMeta.fromJson(asMapOrNull(map['meta'])));
  }
  throw ApiException.unknown(statusCode);
}

/// Builds an [ApiException] from an error body. Never includes the request
/// or response body in the exception (it may hold PII).
ApiException apiExceptionFromBody(Object? body, {int? statusCode, String? requestId}) {
  final err = body is Map ? asMapOrNull(body['error']) : null;
  if (err == null) {
    final code = ApiErrorCode.fromStatus(statusCode);
    return ApiException(
      code: code,
      message: code == ApiErrorCode.network
          ? ApiException.network().message
          : ApiException.unknown().message,
      statusCode: statusCode,
      requestId: requestId,
    );
  }
  final fieldsRaw = asMapOrNull(err['fields']) ?? const <String, dynamic>{};
  final fields = <String, String>{
    for (final e in fieldsRaw.entries)
      if (e.value != null) e.key: '${e.value}',
  };
  final message = asString(err['message']);
  return ApiException(
    code: ApiErrorCode.fromWire(asStringOrNull(err['code'])),
    message: message.isEmpty ? ApiException.unknown().message : message,
    fields: fields,
    statusCode: statusCode,
    requestId: requestId,
  );
}

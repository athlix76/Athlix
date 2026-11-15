import 'json.dart';

/// One page of a cursor-paginated list (`meta.nextCursor`, absent on the last page).
class PageResult<T> {
  const PageResult(this.items, this.nextCursor);

  final List<T> items;
  final String? nextCursor;

  bool get hasMore => nextCursor != null && nextCursor!.isNotEmpty;

  factory PageResult.fromEnvelope(Object? body, T Function(Json) parse) {
    final env = asJson(body);
    final meta = asJson(env['meta']);
    final cursor = asStringOrNull(meta['nextCursor']);
    return PageResult(asList(env['data'], parse), (cursor == null || cursor.isEmpty) ? null : cursor);
  }
}

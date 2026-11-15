import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';
import '../../../core/format/format.dart';
import '../../../core/models/listing.dart';

const listingOrderStatuses = ['created', 'paid', 'failed', 'expired', 'refunded', 'partially_refunded'];

class ListingOrderFilters {
  const ListingOrderFilters({this.status, this.eventId, this.from, this.to});

  final String? status;
  final int? eventId;
  final DateTime? from;
  final DateTime? to;

  Map<String, Object?> toQuery() => {
        'status': status,
        'eventId': eventId,
        'from': from == null ? null : apiDate(from!.toLocal()),
        'to': to == null ? null : apiDate(to!.toLocal()),
      };
}

class ListingOrdersApi {
  ListingOrdersApi(this._client);

  final ApiClient _client;

  /// `GET /admin/listing-orders` (payments.read).
  Future<PageResult<ListingOrder>> list(ListingOrderFilters f, String? cursor) =>
      _client.page('/admin/listing-orders', ListingOrder.fromJson, query: f.toQuery(), cursor: cursor);

  /// `POST /admin/listing-orders/:id/refund` (payments.refund). Amount in paise.
  Future<ListingOrder> refund(int orderId, int amountPaise, String reason) async => ListingOrder.fromJson(asJson(
      await _client.post('/admin/listing-orders/$orderId/refund', body: {'amountPaise': amountPaise, 'reason': reason})));
}

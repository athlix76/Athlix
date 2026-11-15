import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import 'listing_models.dart';

class ListingApi {
  const ListingApi(this._api);

  final ApiClient _api;

  /// Calculator before the event exists (`GET /listing/quote`).
  Future<ListingQuote> estimate({
    required int sportId,
    required String eventType,
    required int entryFeePaise,
    required int maxSlots,
  }) =>
      _api.get(
        '/listing/quote',
        (d) => ListingQuote.fromJson(asMap(d)),
        query: {
          'sportId': sportId,
          'eventType': eventType,
          'entryFeePaise': entryFeePaise,
          'maxSlots': maxSlots,
        },
      );

  Future<ListingQuote> quote(int eventId) =>
      _api.post('/events/$eventId/listing/quote', (d) => ListingQuote.fromJson(asMap(d)));

  Future<ListingOrder> createOrder(int eventId) =>
      _api.post('/events/$eventId/listing/order', (d) => ListingOrder.fromJson(asMap(d)));

  /// Latest order for the caller's event and its status (`GET /events/:id/listing/order`); 404 when none.
  Future<ListingOrder> latestOrder(int eventId) =>
      _api.get('/events/$eventId/listing/order', (d) => ListingOrder.fromJson(asMap(d)));

  Future<ListingOrder> confirm(
    int eventId, {
    required int orderId,
    required String providerPaymentId,
    required String providerSignature,
  }) =>
      _api.post(
        '/events/$eventId/listing/confirm',
        (d) => ListingOrder.fromJson(asMap(d)),
        body: {
          'orderId': orderId,
          'providerPaymentId': providerPaymentId,
          'providerSignature': providerSignature,
        },
      );

  /// Non-prod only (`listing.payment_provider` = mock).
  Future<ListingOrder> mockPay(int orderId) => _api.post(
        '/listing/mock/pay',
        (d) => ListingOrder.fromJson(asMap(d)),
        body: {'orderId': orderId},
      );
}

final listingApiProvider = Provider<ListingApi>((ref) => ListingApi(ref.watch(apiClientProvider)));

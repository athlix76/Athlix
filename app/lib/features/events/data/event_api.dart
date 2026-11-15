import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/envelope.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import '../../location_identity/data/eligibility_models.dart';
import 'event_models.dart';

class EventApi {
  const EventApi(this._api);

  final ApiClient _api;

  Future<Event> create(EventInput input) =>
      _api.post('/events', (d) => Event.fromJson(asMap(d)), body: input.toJson());

  Future<Event> update(int id, Map<String, dynamic> body) =>
      _api.put('/events/$id', (d) => Event.fromJson(asMap(d)), body: body);

  Future<Event> get(int id) => _api.get('/events/$id', (d) => Event.fromJson(asMap(d)));

  Future<Paged<EventSummary>> list(EventFilters filters, {String? cursor}) => _api.page(
        '/events',
        EventSummary.fromJson,
        query: {...filters.toQuery(), 'cursor': cursor},
      );

  Future<List<EventSummary>> featured({int? sportId, String? state}) => _api.get(
        '/events/featured',
        (d) => asList(d, EventSummary.fromJson),
        query: {'sportId': sportId, 'state': state},
      );

  Future<Paged<EventSummary>> mine({String? status, String? cursor}) => _api.page(
        '/me/events',
        EventSummary.fromJson,
        query: {'status': status, 'cursor': cursor},
      );

  /// Organiser form preview line (`GET /events/eligibility/preview`).
  Future<EligibilityPreview> eligibilityPreview(String category, EligibilityInput rule) => _api.get(
        '/events/eligibility/preview',
        (d) => EligibilityPreview.fromJson(asMap(d)),
        query: rule.toPreviewQuery(category),
      );

  Future<EventContact> contact(int id) =>
      _api.get('/events/$id/contact', (d) => EventContact.fromJson(asMap(d)));

  Future<Event> cancel(int id, String reason) =>
      _api.post('/events/$id/cancel', (d) => Event.fromJson(asMap(d)), body: {'reason': reason});

  Future<Event> feature(int id) => _api.post('/events/$id/feature', (d) => Event.fromJson(asMap(d)));
}

final eventApiProvider = Provider<EventApi>((ref) => EventApi(ref.watch(apiClientProvider)));

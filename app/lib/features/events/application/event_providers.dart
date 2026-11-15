import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../listing_payment/data/listing_api.dart';
import '../../location_identity/data/eligibility_models.dart';
import '../../listing_payment/data/listing_models.dart';
import '../data/event_api.dart';
import '../data/event_models.dart';

final eventProvider = FutureProvider.autoDispose.family<Event, int>(
  (ref, id) => ref.watch(eventApiProvider).get(id),
);

final featuredEventsProvider = FutureProvider.autoDispose<List<EventSummary>>(
  (ref) => ref.watch(eventApiProvider).featured(),
);

/// Filters of the events tab (kept while the app runs).
final eventFiltersProvider = StateProvider<EventFilters>((ref) => const EventFilters());

/// Key for the live listing-fee estimate in the create form.
/// Inputs of the eligibility preview; a record so equal inputs share a request.
typedef EligibilityPreviewKey = ({
  String category,
  String type,
  String? scope,
  int? districtId,
  int? stateId,
  bool requireVerified,
});

final eligibilityPreviewProvider =
    FutureProvider.autoDispose.family<EligibilityPreview, EligibilityPreviewKey>(
  (ref, k) => ref.watch(eventApiProvider).eligibilityPreview(
        k.category,
        EligibilityInput(
          type: k.type,
          scope: k.scope,
          districtId: k.districtId,
          stateId: k.stateId,
          requireVerified: k.requireVerified,
        ),
      ),
);

typedef QuoteKey = ({int sportId, String eventType, int entryFeePaise, int maxSlots});

final listingEstimateProvider = FutureProvider.autoDispose.family<ListingQuote, QuoteKey>(
  (ref, k) => ref.watch(listingApiProvider).estimate(
        sportId: k.sportId,
        eventType: k.eventType,
        entryFeePaise: k.entryFeePaise,
        maxSlots: k.maxSlots,
      ),
);

/// Writes for events. Every write refreshes the cached event.
class EventController {
  EventController(this._ref);

  final Ref _ref;

  EventApi get _api => _ref.read(eventApiProvider);

  Future<Event> create(EventInput input) => _api.create(input);

  /// Published events accept only [EventEnums.publishedEditable].
  Future<Event> update(int id, EventInput input, {required String status}) async {
    final body = status == 'published'
        ? input.toJson(only: EventEnums.publishedEditable)
        : input.toJson();
    final e = await _api.update(id, body);
    _ref.invalidate(eventProvider(id));
    return e;
  }

  Future<Event> cancel(int id, String reason) async {
    final e = await _api.cancel(id, reason);
    _ref.invalidate(eventProvider(id));
    return e;
  }

  Future<Event> feature(int id) async {
    final e = await _api.feature(id);
    _ref.invalidate(eventProvider(id));
    return e;
  }

  /// Not cached: the full number is shown once and dropped with the sheet.
  Future<EventContact> contact(int id) => _api.contact(id);
}

final eventControllerProvider = Provider<EventController>(EventController.new);

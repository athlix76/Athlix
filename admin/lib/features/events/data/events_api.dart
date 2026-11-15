import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';
import '../../../core/models/common.dart';
import '../../../core/models/listing.dart';

const eventStatuses = [
  'draft',
  'pending_payment',
  'pending_review',
  'published',
  'rejected',
  'removed',
  'cancelled',
  'completed',
];

/// `EventSummary` (+ `listing` in the admin list) and, for the detail view,
/// the extra `Event` fields kept as raw JSON for display.
class AdminEvent {
  const AdminEvent({
    required this.id,
    required this.title,
    this.sport,
    required this.eventType,
    required this.status,
    required this.isFeatured,
    this.organiser,
    this.city,
    this.state,
    this.venueName,
    this.startsAt,
    this.registrationDeadline,
    required this.entryFeePaise,
    required this.maxSlots,
    required this.registrationsCount,
    this.bannerUrl,
    this.listing,
    required this.raw,
    this.listingOrders = const [],
  });

  final int id;
  final String title;
  final SportRef? sport;
  final String eventType;
  final String status;
  final bool isFeatured;
  final UserSummary? organiser;
  final String? city;
  final String? state;
  final String? venueName;
  final DateTime? startsAt;
  final DateTime? registrationDeadline;
  final int entryFeePaise;
  final int maxSlots;
  final int registrationsCount;
  final String? bannerUrl;

  /// `{orderId, status, totalPaise}` or null.
  final Json? listing;
  final Json raw;
  final List<ListingOrder> listingOrders;

  factory AdminEvent.fromJson(Json j) => AdminEvent(
        id: asInt(j['id']),
        title: asString(j['title']),
        sport: SportRef.maybe(j['sport']),
        eventType: asString(j['eventType']),
        status: asString(j['status']),
        isFeatured: asBool(j['isFeatured']),
        organiser: UserSummary.maybe(j['organiser']),
        city: asStringOrNull(j['city']),
        state: asStringOrNull(j['state']),
        venueName: asStringOrNull(j['venueName']),
        startsAt: asDate(j['startsAt']),
        registrationDeadline: asDate(j['registrationDeadline']),
        entryFeePaise: asInt(j['entryFeePaise']),
        maxSlots: asInt(j['maxSlots']),
        registrationsCount: asInt(j['registrationsCount']),
        bannerUrl: asStringOrNull(j['bannerUrl']),
        listing: asJsonOrNull(j['listing']),
        raw: j,
        listingOrders: asList(j['listingOrders'], ListingOrder.fromJson),
      );
}

class EventsApi {
  EventsApi(this._client);

  final ApiClient _client;

  /// `GET /admin/events` (events.review).
  Future<PageResult<AdminEvent>> list({String? status, int? sportId, String? q, String? cursor}) => _client.page(
        '/admin/events',
        AdminEvent.fromJson,
        query: {'status': status, 'sportId': sportId, 'q': q},
        cursor: cursor,
      );

  /// `GET /admin/events/:id` (events.review).
  Future<AdminEvent> detail(int id) async => AdminEvent.fromJson(asJson(await _client.get('/admin/events/$id')));

  /// `POST /admin/events/:id/approve`.
  Future<AdminEvent> approve(int id) async => AdminEvent.fromJson(asJson(await _client.post('/admin/events/$id/approve', body: const {})));

  /// `POST /admin/events/:id/reject` (listing refund per config).
  Future<AdminEvent> reject(int id, String reason) async =>
      AdminEvent.fromJson(asJson(await _client.post('/admin/events/$id/reject', body: {'reason': reason})));

  /// `POST /admin/events/:id/feature` / `unfeature` (events.review): pins a
  /// published event to the top of discovery, or unpins it.
  Future<AdminEvent> setFeatured(int id, bool on) async => AdminEvent.fromJson(asJson(on
      ? await _client.post('/admin/events/$id/feature', body: const {})
      : await _client.post('/admin/events/$id/unfeature', body: const {})));

  /// `GET /admin/events/:id/registrations` (events.review): the organiser's
  /// view, with how each player qualified (`qualifiedLabel`).
  Future<PageResult<AdminRegistration>> registrations(int id, {String? status, bool flaggedOnly = false, String? cursor}) =>
      _client.page(
        '/admin/events/$id/registrations',
        AdminRegistration.fromJson,
        query: {'status': status, if (flaggedOnly) 'flagged': 'true'},
        cursor: cursor,
      );

  /// `POST /admin/events/:id/remove` (published → removed).
  Future<AdminEvent> remove(int id, String reason) async =>
      AdminEvent.fromJson(asJson(await _client.post('/admin/events/$id/remove', body: {'reason': reason})));
}

/// One row of `GET /admin/events/:id/registrations`.
class AdminRegistration {
  const AdminRegistration({
    required this.id,
    this.user,
    required this.status,
    required this.paymentStatus,
    this.teamName,
    required this.qualifiedLabel,
    required this.eligibilityFlag,
    this.createdAt,
  });

  final int id;
  final UserSummary? user;
  final String status;
  final String paymentStatus;
  final String? teamName;
  final String qualifiedLabel;

  /// `none` unless the player's eligibility changed after registering.
  final String eligibilityFlag;
  final DateTime? createdAt;

  factory AdminRegistration.fromJson(Json j) => AdminRegistration(
        id: asInt(j['id']),
        user: UserSummary.maybe(j['user']),
        status: asString(j['status']),
        paymentStatus: asString(j['paymentStatus']),
        teamName: asStringOrNull(j['teamName']),
        qualifiedLabel: asString(j['qualifiedLabel']),
        eligibilityFlag: asString(j['eligibilityFlag'], 'none'),
        createdAt: asDate(j['createdAt']),
      );
}

import '../../../core/json/json_utils.dart';
import '../../../core/location/device_location.dart';
import '../../../core/models/sport.dart';
import '../../../core/models/user_summary.dart';
import '../../location_identity/data/eligibility_models.dart';
import '../../registrations/data/registration_models.dart';

/// Enum values from docs/api.md "Events".
abstract final class EventEnums {
  static const eventTypes = ['tournament', 'trial', 'camp', 'match', 'other'];
  static const participantTypes = ['individual', 'team'];
  static const competitionLevels = [
    'open', 'school', 'college', 'club', 'district', 'state', 'national', 'other' //
  ];
  static const genderCategories = ['open', 'male', 'female', 'mixed'];
  static const paymentModes = ['cash', 'upi', 'bank_transfer', 'online_link'];
  static const myEventStatuses = [
    'draft', 'pending_payment', 'pending_review', 'published', 'completed', 'cancelled', //
    'rejected', 'removed'
  ];

  /// Statuses the organiser may still cancel from.
  static const cancellable = ['draft', 'pending_payment', 'pending_review', 'published'];

  /// Statuses where a listing order can be started.
  static const payable = ['draft', 'pending_payment', 'rejected'];

  /// Fields that stay editable once `published`.
  static const publishedEditable = {
    'description', 'rules', 'prizeInfo', 'paymentInstructions', 'bannerUploadId', //
    'contactName', 'contactPhone', 'venueName', 'addressLine'
  };
}

class EventSummary {
  const EventSummary({
    required this.id,
    required this.title,
    required this.sport,
    required this.eventType,
    required this.participantType,
    required this.competitionLevel,
    required this.genderCategory,
    this.ageMin,
    this.ageMax,
    this.venueName,
    this.city,
    this.district,
    this.state,
    this.startsAt,
    this.endsAt,
    this.registrationDeadline,
    required this.entryFeePaise,
    required this.maxSlots,
    required this.registrationsCount,
    required this.slotsLeft,
    this.bannerUrl,
    required this.status,
    required this.isFeatured,
    this.organiser,
    this.category = EventCategory.private,
    this.eligibility,
    this.myEligibility,
    this.distanceKm,
  });

  final int id;
  final String title;
  final SportRef? sport;
  final String eventType;
  final String participantType;
  final String competitionLevel;
  final String genderCategory;
  final int? ageMin;
  final int? ageMax;
  final String? venueName;
  final String? city;
  final String? district;
  final String? state;
  final DateTime? startsAt;
  final DateTime? endsAt;
  final DateTime? registrationDeadline;
  final int entryFeePaise;
  final int maxSlots;
  final int registrationsCount;
  final int slotsLeft;
  final String? bannerUrl;
  final String status;
  final bool isFeatured;
  final UserSummary? organiser;
  final String category;

  /// Null when the server sent none (feature off): treat as open.
  final EligibilityRule? eligibility;

  /// The caller's standing; null when the feature is off.
  final MyEligibility? myEligibility;

  /// Only when `lat`/`lng` were sent with the list request.
  final double? distanceKm;

  bool get isEligible => myEligibility?.eligible ?? true;

  bool get isTeam => participantType == 'team';
  bool get isPublished => status == 'published';

  bool registrationOpen(DateTime now) =>
      isPublished && (registrationDeadline == null || registrationDeadline!.isAfter(now));

  String get place => [venueName, city, state].whereType<String>().where((s) => s.isNotEmpty).join(', ');

  String get ageText {
    if (ageMin == null && ageMax == null) return 'All ages';
    if (ageMin == null) return 'Up to $ageMax years';
    if (ageMax == null) return '$ageMin+ years';
    return '$ageMin to $ageMax years';
  }

  factory EventSummary.fromJson(Map<String, dynamic> json) => EventSummary(
        id: asInt(json['id']),
        title: asString(json['title']),
        sport: SportRef.maybe(json['sport']),
        eventType: asString(json['eventType']),
        participantType: asString(json['participantType'], 'individual'),
        competitionLevel: asString(json['competitionLevel']),
        genderCategory: asString(json['genderCategory'], 'open'),
        ageMin: asIntOrNull(json['ageMin']),
        ageMax: asIntOrNull(json['ageMax']),
        venueName: asStringOrNull(json['venueName']),
        city: asStringOrNull(json['city']),
        district: asStringOrNull(json['district']),
        state: asStringOrNull(json['state']),
        startsAt: asDateTimeOrNull(json['startsAt']),
        endsAt: asDateTimeOrNull(json['endsAt']),
        registrationDeadline: asDateTimeOrNull(json['registrationDeadline']),
        entryFeePaise: asInt(json['entryFeePaise']),
        maxSlots: asInt(json['maxSlots']),
        registrationsCount: asInt(json['registrationsCount']),
        slotsLeft: asInt(json['slotsLeft']),
        bannerUrl: asStringOrNull(json['bannerUrl']),
        status: asString(json['status']),
        isFeatured: asBool(json['isFeatured']),
        organiser: UserSummary.maybe(json['organiser']),
        category: asString(json['category'], EventCategory.private),
        eligibility: EligibilityRule.maybe(json['eligibility']),
        myEligibility: MyEligibility.maybe(json['myEligibility']),
        distanceKm: asNumOrNull(json['distanceKm'])?.toDouble(),
      );
}

/// `Event.listing` (owner only).
class EventListingRef {
  const EventListingRef({required this.orderId, required this.status, required this.totalPaise});

  final int orderId;
  final String status;
  final int totalPaise;

  static EventListingRef? maybe(Object? v) {
    final m = asMapOrNull(v);
    if (m == null) return null;
    return EventListingRef(
      orderId: asInt(m['orderId']),
      status: asString(m['status']),
      totalPaise: asInt(m['totalPaise']),
    );
  }
}

/// `Event` = `EventSummary` + detail fields.
class Event {
  const Event({
    required this.summary,
    this.description,
    this.teamSizeMin,
    this.teamSizeMax,
    this.addressLine,
    this.pincode,
    this.latitude,
    this.longitude,
    required this.paymentModes,
    this.paymentInstructions,
    this.rules,
    this.prizeInfo,
    this.contactName,
    this.contactPhoneMasked,
    this.rejectionReason,
    this.cancelReason,
    this.publishedAt,
    required this.isOwner,
    this.myRegistration,
    required this.hasGroupChat,
    this.listing,
  });

  final EventSummary summary;
  final String? description;
  final int? teamSizeMin;
  final int? teamSizeMax;
  final String? addressLine;
  final String? pincode;
  final double? latitude;
  final double? longitude;
  final List<String> paymentModes;
  final String? paymentInstructions;
  final String? rules;
  final String? prizeInfo;
  final String? contactName;
  final String? contactPhoneMasked;
  final String? rejectionReason;
  final String? cancelReason;
  final DateTime? publishedAt;
  final bool isOwner;
  final Registration? myRegistration;
  final bool hasGroupChat;
  final EventListingRef? listing;

  int get id => summary.id;
  String get status => summary.status;

  factory Event.fromJson(Map<String, dynamic> json) => Event(
        summary: EventSummary.fromJson(json),
        description: asStringOrNull(json['description']),
        teamSizeMin: asIntOrNull(json['teamSizeMin']),
        teamSizeMax: asIntOrNull(json['teamSizeMax']),
        addressLine: asStringOrNull(json['addressLine']),
        pincode: asStringOrNull(json['pincode']),
        latitude: asNumOrNull(json['latitude'])?.toDouble(),
        longitude: asNumOrNull(json['longitude'])?.toDouble(),
        paymentModes: asStringList(json['paymentModes']),
        paymentInstructions: asStringOrNull(json['paymentInstructions']),
        rules: asStringOrNull(json['rules']),
        prizeInfo: asStringOrNull(json['prizeInfo']),
        contactName: asStringOrNull(json['contactName']),
        contactPhoneMasked: asStringOrNull(json['contactPhoneMasked']),
        rejectionReason: asStringOrNull(json['rejectionReason']),
        cancelReason: asStringOrNull(json['cancelReason']),
        publishedAt: asDateTimeOrNull(json['publishedAt']),
        isOwner: asBool(json['isOwner']),
        myRegistration: asMapOrNull(json['myRegistration']) == null
            ? null
            : Registration.fromJson(asMap(json['myRegistration'])),
        hasGroupChat: asBool(json['hasGroupChat']),
        listing: EventListingRef.maybe(json['listing']),
      );
}

/// Body of `POST /events` and `PUT /events/:id`.
class EventInput {
  EventInput({
    this.sportId,
    this.title = '',
    this.description,
    this.eventType = 'tournament',
    this.participantType = 'individual',
    this.teamSizeMin,
    this.teamSizeMax,
    this.competitionLevel = 'open',
    this.genderCategory = 'open',
    this.ageMin,
    this.ageMax,
    this.venueName,
    this.addressLine,
    this.city,
    this.district,
    this.state,
    this.pincode,
    this.startsAt,
    this.endsAt,
    this.registrationDeadline,
    this.entryFeePaise,
    List<String>? paymentModes,
    this.paymentInstructions,
    this.maxSlots,
    this.rules,
    this.prizeInfo,
    this.contactName,
    this.contactPhone,
    this.bannerUploadId,
    this.category = EventCategory.private,
    EligibilityInput? eligibility,
  })  : paymentModes = paymentModes ?? <String>[],
        eligibility = eligibility ?? EligibilityInput();

  int? sportId;
  String title;
  String? description;
  String eventType;
  String participantType;
  int? teamSizeMin;
  int? teamSizeMax;
  String competitionLevel;
  String genderCategory;
  int? ageMin;
  int? ageMax;
  String? venueName;
  String? addressLine;
  String? city;
  String? district;
  String? state;
  String? pincode;
  DateTime? startsAt;
  DateTime? endsAt;
  DateTime? registrationDeadline;
  int? entryFeePaise;
  List<String> paymentModes;
  String? paymentInstructions;
  int? maxSlots;
  String? rules;
  String? prizeInfo;
  String? contactName;

  /// Full number typed by the organiser. Only sent, never logged.
  String? contactPhone;
  int? bannerUploadId;

  /// Locked once published (not in [EventEnums.publishedEditable]).
  String category;
  EligibilityInput eligibility;

  /// Prefills the form from an existing event. The full contact phone is not
  /// returned by `GET /events/:id`, so it stays empty (unchanged unless typed).
  factory EventInput.fromEvent(Event e) => EventInput(
        sportId: e.summary.sport?.id,
        title: e.summary.title,
        description: e.description,
        eventType: e.summary.eventType,
        participantType: e.summary.participantType,
        teamSizeMin: e.teamSizeMin,
        teamSizeMax: e.teamSizeMax,
        competitionLevel: e.summary.competitionLevel,
        genderCategory: e.summary.genderCategory,
        ageMin: e.summary.ageMin,
        ageMax: e.summary.ageMax,
        venueName: e.summary.venueName,
        addressLine: e.addressLine,
        city: e.summary.city,
        district: e.summary.district,
        state: e.summary.state,
        pincode: e.pincode,
        startsAt: e.summary.startsAt,
        endsAt: e.summary.endsAt,
        registrationDeadline: e.summary.registrationDeadline,
        entryFeePaise: e.summary.entryFeePaise,
        paymentModes: List<String>.of(e.paymentModes),
        paymentInstructions: e.paymentInstructions,
        maxSlots: e.summary.maxSlots,
        rules: e.rules,
        prizeInfo: e.prizeInfo,
        contactName: e.contactName,
        category: e.summary.category,
        eligibility: EligibilityInput.fromRule(e.summary.eligibility),
      );

  /// [only] limits the body to those keys (published events accept only
  /// [EventEnums.publishedEditable]). Null values are dropped from partial
  /// bodies so an unchanged field is never cleared by accident.
  Map<String, dynamic> toJson({Set<String>? only}) {
    final all = <String, dynamic>{
      'sportId': sportId,
      'title': title,
      'description': description,
      'eventType': eventType,
      'participantType': participantType,
      'teamSizeMin': participantType == 'team' ? teamSizeMin : null,
      'teamSizeMax': participantType == 'team' ? teamSizeMax : null,
      'competitionLevel': competitionLevel,
      'genderCategory': genderCategory,
      'ageMin': ageMin,
      'ageMax': ageMax,
      'venueName': venueName,
      'addressLine': addressLine,
      'city': city,
      'district': district,
      'state': state,
      'pincode': pincode,
      'startsAt': formatRfc3339OrNull(startsAt),
      'endsAt': formatRfc3339OrNull(endsAt),
      'registrationDeadline': formatRfc3339OrNull(registrationDeadline),
      'entryFeePaise': entryFeePaise,
      'paymentModes': paymentModes,
      'paymentInstructions': paymentInstructions,
      'maxSlots': maxSlots,
      'rules': rules,
      'prizeInfo': prizeInfo,
      'contactName': contactName,
      'contactPhone': contactPhone,
      'bannerUploadId': bannerUploadId,
      'category': category,
      'eligibility': eligibility.toJson(),
    };
    if (only == null) {
      all.removeWhere((k, v) => v == null && (k == 'contactPhone' || k == 'bannerUploadId'));
      return all;
    }
    return {
      for (final e in all.entries)
        if (only.contains(e.key) && e.value != null) e.key: e.value,
    };
  }

  @override
  String toString() => 'EventInput(<redacted>)';
}

/// `GET /events/:id/contact`: full number so players can call the organiser.
class EventContact {
  const EventContact({
    required this.contactName,
    required this.contactPhone,
    required this.paymentModes,
    this.paymentInstructions,
  });

  final String contactName;
  final String contactPhone;
  final List<String> paymentModes;
  final String? paymentInstructions;

  factory EventContact.fromJson(Map<String, dynamic> json) => EventContact(
        contactName: asString(json['contactName']),
        contactPhone: asString(json['contactPhone']),
        paymentModes: asStringList(json['paymentModes']),
        paymentInstructions: asStringOrNull(json['paymentInstructions']),
      );

  @override
  String toString() => 'EventContact(<redacted>)';
}

/// `GET /events` query.
class EventFilters {
  const EventFilters({
    this.q,
    this.sportId,
    this.eventType,
    this.competitionLevel,
    this.genderCategory,
    this.state,
    this.district,
    this.city,
    this.fromDate,
    this.toDate,
    this.maxEntryFeePaise,
    this.eligibleForMe = false,
    this.includeClosed = false,
    this.category,
    this.showAll = false,
    this.near,
    this.radiusKm,
  });

  final String? q;
  final int? sportId;
  final String? eventType;
  final String? competitionLevel;
  final String? genderCategory;
  final String? state;
  final String? district;
  final String? city;
  final DateTime? fromDate;
  final DateTime? toDate;
  final int? maxEntryFeePaise;
  final bool eligibleForMe;
  final bool includeClosed;
  final String? category;

  /// Also return events the caller is not eligible for (shown greyed).
  final bool showAll;

  /// Device location for the distance filter. In memory only, never stored.
  final LatLng? near;
  final int? radiusKm;

  bool get byDistance => near != null && radiusKm != null;

  int get activeCount => [
        sportId, eventType, competitionLevel, genderCategory, state, district, city, //
        fromDate, toDate, maxEntryFeePaise
      ].where((v) => v != null && v != '').length +
      (eligibleForMe ? 1 : 0) +
      (includeClosed ? 1 : 0) +
      (category != null ? 1 : 0);

  Map<String, Object?> toQuery() => {
        'q': q,
        'sportId': sportId,
        'eventType': eventType,
        'competitionLevel': competitionLevel,
        'genderCategory': genderCategory,
        'state': state,
        'district': district,
        'city': city,
        'fromDate': fromDate == null ? null : formatDateOnly(fromDate!),
        'toDate': toDate == null ? null : formatDateOnly(toDate!),
        'maxEntryFeePaise': maxEntryFeePaise,
        'eligibleForMe': eligibleForMe ? true : null,
        'includeClosed': includeClosed ? true : null,
        'category': category,
        'showAll': showAll ? true : null,
        'lat': byDistance ? near!.lat : null,
        'lng': byDistance ? near!.lng : null,
        'radiusKm': byDistance ? radiusKm : null,
        'sort': byDistance ? 'distance' : null,
      };

  EventFilters withQuery(String? text) => copyWith(q: () => text);

  /// Nullable fields take a thunk so they can be cleared: `copyWith(near: () => null)`.
  EventFilters copyWith({
    String? Function()? q,
    String? Function()? category,
    bool? showAll,
    LatLng? Function()? near,
    int? Function()? radiusKm,
    bool? eligibleForMe,
    bool? includeClosed,
  }) =>
      EventFilters(
        q: q == null ? this.q : q(),
        sportId: sportId,
        eventType: eventType,
        competitionLevel: competitionLevel,
        genderCategory: genderCategory,
        state: state,
        district: district,
        city: city,
        fromDate: fromDate,
        toDate: toDate,
        maxEntryFeePaise: maxEntryFeePaise,
        eligibleForMe: eligibleForMe ?? this.eligibleForMe,
        includeClosed: includeClosed ?? this.includeClosed,
        category: category == null ? this.category : category(),
        showAll: showAll ?? this.showAll,
        near: near == null ? this.near : near(),
        radiusKm: radiusKm == null ? this.radiusKm : radiusKm(),
      );

  @override
  bool operator ==(Object other) =>
      other is EventFilters && _key == other._key;

  @override
  int get hashCode => _key.hashCode;

  String get _key => toQuery().toString();
}

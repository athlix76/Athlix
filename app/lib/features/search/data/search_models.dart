import '../../../core/json/json_utils.dart';
import '../../../core/models/sport.dart';
import '../../../core/models/user_summary.dart';
import '../../events/data/event_models.dart';

/// `PersonResult` = UserSummary + `{bio, followersCount}` (docs/api.md, Search).
/// For minors the server already drops photo, city and bio unless the viewer is a mutual follow.
class PersonResult {
  const PersonResult({required this.user, required this.bio, required this.followersCount});

  final UserSummary user;
  final String? bio;
  final int followersCount;

  factory PersonResult.fromJson(Map<String, dynamic> json) => PersonResult(
        user: UserSummary.fromJson(json),
        bio: asStringOrNull(json['bio']),
        followersCount: asInt(json['followersCount']),
      );
}

/// The fields of `Community` that search shows. The full model lives in features/communities;
/// this copy keeps search independent of it.
class CommunityHit {
  const CommunityHit({
    required this.id,
    required this.title,
    required this.description,
    required this.avatarUrl,
    required this.sport,
    required this.city,
    required this.state,
    required this.memberCount,
  });

  final int id;
  final String title;
  final String? description;
  final String? avatarUrl;
  final SportRef? sport;
  final String? city;
  final String? state;
  final int memberCount;

  factory CommunityHit.fromJson(Map<String, dynamic> json) => CommunityHit(
        id: asInt(json['id']),
        title: asString(json['title']),
        description: asStringOrNull(json['description']),
        avatarUrl: asStringOrNull(json['avatarUrl']),
        sport: SportRef.maybe(json['sport']),
        city: asStringOrNull(json['city']),
        state: asStringOrNull(json['state']),
        memberCount: asInt(json['memberCount']),
      );

  String get subtitle => [
        if (sport != null) sport!.name,
        if (city != null && city!.isNotEmpty) city!,
        '$memberCount member${memberCount == 1 ? '' : 's'}',
      ].join(' · ');
}

/// `GET /search?q` (max 5 of each).
class SearchResults {
  const SearchResults({required this.people, required this.events, required this.communities});

  static const empty = SearchResults(people: [], events: [], communities: []);

  final List<UserSummary> people;
  final List<EventSummary> events;
  final List<CommunityHit> communities;

  bool get isEmpty => people.isEmpty && events.isEmpty && communities.isEmpty;

  factory SearchResults.fromJson(Map<String, dynamic> json) => SearchResults(
        people: asList(json['people'], UserSummary.fromJson),
        events: asList(json['events'], EventSummary.fromJson),
        communities: asList(json['communities'], CommunityHit.fromJson),
      );
}

/// `achievementLevel` values (same list as AchievementInput.level).
abstract final class AchievementLevels {
  static const all = ['school', 'college', 'club', 'district', 'state', 'national', 'international', 'other'];
}

/// Filters of `GET /search/people`. [ageMin], [ageMax] and [achievementLevel] are advanced:
/// without benefit `advanced_search_filters` the server answers 403 `PLAN_REQUIRED`.
class PeopleFilters {
  const PeopleFilters({
    this.q,
    this.role,
    this.sportId,
    this.positionId,
    this.state,
    this.district,
    this.city,
    this.ageMin,
    this.ageMax,
    this.achievementLevel,
  });

  final String? q;
  final String? role;
  final int? sportId;
  final int? positionId;
  final String? state;
  final String? district;
  final String? city;
  final int? ageMin;
  final int? ageMax;
  final String? achievementLevel;

  bool get usesAdvanced => ageMin != null || ageMax != null || achievementLevel != null;

  /// Number of filters set besides [q] (for the filter button badge).
  int get activeCount => [role, sportId, positionId, state, district, city, ageMin, ageMax, achievementLevel]
      .where((v) => v != null && v != '')
      .length;

  PeopleFilters withQuery(String? value) => PeopleFilters(
        q: value,
        role: role,
        sportId: sportId,
        positionId: positionId,
        state: state,
        district: district,
        city: city,
        ageMin: ageMin,
        ageMax: ageMax,
        achievementLevel: achievementLevel,
      );

  Map<String, Object?> toQuery() => {
        'q': q,
        'role': role,
        'sportId': sportId,
        'positionId': positionId,
        'state': state,
        'district': district,
        'city': city,
        'ageMin': ageMin,
        'ageMax': ageMax,
        'achievementLevel': achievementLevel,
      };

  /// Stable key so a changed filter set reloads the list.
  String get cacheKey => toQuery().entries.where((e) => e.value != null).map((e) => '${e.key}=${e.value}').join('&');
}

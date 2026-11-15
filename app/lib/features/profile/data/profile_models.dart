import '../../../core/json/json_utils.dart';
import '../../../core/models/sport.dart';
import '../../../core/models/user_summary.dart';

/// `visibility` / `statsVisibility` values.
abstract final class ProfileVisibility {
  static const public = 'public';
  static const followers = 'followers';
  static const private = 'private';

  static const all = [public, followers, private];

  static const labels = {public: 'Everyone', followers: 'Followers only', private: 'Only me'};
}

/// `AchievementInput.level` values.
abstract final class AchievementLevel {
  static const all = ['school', 'college', 'club', 'district', 'state', 'national', 'international', 'other'];
}

/// One sport on a profile.
class ProfileSport {
  const ProfileSport({required this.sport, required this.position, required this.isPrimary});

  final SportRef sport;
  final SportRef? position;
  final bool isPrimary;

  factory ProfileSport.fromJson(Map<String, dynamic> json) => ProfileSport(
        sport: SportRef.fromJson(asMap(json['sport'])),
        position: SportRef.maybe(json['position']),
        isPrimary: asBool(json['isPrimary']),
      );
}

class OrganiserInfo {
  const OrganiserInfo({required this.status, required this.orgName, required this.orgType});

  final String status;
  final String? orgName;
  final String? orgType;

  factory OrganiserInfo.fromJson(Map<String, dynamic> json) => OrganiserInfo(
        status: asString(json['status']),
        orgName: asStringOrNull(json['orgName']),
        orgType: asStringOrNull(json['orgType']),
      );
}

/// `Profile` from docs/api.md. When [restricted] is true only [user], [isOwn],
/// [restricted], the follow flags and the counts are filled.
class Profile {
  const Profile({
    required this.user,
    required this.isOwn,
    required this.restricted,
    required this.bio,
    required this.sports,
    required this.district,
    required this.academyClub,
    required this.birthYear,
    required this.visibility,
    required this.statsVisibility,
    required this.followersCount,
    required this.followingCount,
    required this.isFollowing,
    required this.isFollowedBy,
    required this.isBlockedByMe,
    required this.canMessage,
    required this.badges,
    required this.coachingSinceYear,
    required this.consultationOffered,
    required this.organiser,
  });

  final UserSummary user;
  final bool isOwn;
  final bool restricted;
  final String? bio;
  final List<ProfileSport> sports;
  final String? district;
  final String? academyClub;

  /// Only sent when [isOwn].
  final int? birthYear;
  final String? visibility;
  final String? statsVisibility;
  final int followersCount;
  final int followingCount;
  final bool isFollowing;
  final bool isFollowedBy;
  final bool isBlockedByMe;
  final bool canMessage;
  final List<String> badges;
  final int? coachingSinceYear;

  /// Coaches only; null for everyone else.
  final bool? consultationOffered;
  final OrganiserInfo? organiser;

  ProfileSport? get primarySport {
    for (final s in sports) {
      if (s.isPrimary) return s;
    }
    return sports.isEmpty ? null : sports.first;
  }

  factory Profile.fromJson(Map<String, dynamic> json) {
    final consultation = asMapOrNull(json['consultation']);
    final organiser = asMapOrNull(json['organiser']);
    return Profile(
      user: UserSummary.fromJson(asMap(json['user'])),
      isOwn: asBool(json['isOwn']),
      restricted: asBool(json['restricted']),
      bio: asStringOrNull(json['bio']),
      sports: asList(json['sports'], ProfileSport.fromJson),
      district: asStringOrNull(json['district']),
      academyClub: asStringOrNull(json['academyClub']),
      birthYear: asIntOrNull(json['birthYear']),
      visibility: asStringOrNull(json['visibility']),
      statsVisibility: asStringOrNull(json['statsVisibility']),
      followersCount: asInt(json['followersCount']),
      followingCount: asInt(json['followingCount']),
      isFollowing: asBool(json['isFollowing']),
      isFollowedBy: asBool(json['isFollowedBy']),
      isBlockedByMe: asBool(json['isBlockedByMe']),
      canMessage: asBool(json['canMessage']),
      badges: asStringList(json['badges']),
      coachingSinceYear: asIntOrNull(json['coachingSinceYear']),
      consultationOffered: consultation == null ? null : asBool(consultation['offered']),
      organiser: organiser == null ? null : OrganiserInfo.fromJson(organiser),
    );
  }

  Profile copyWith({bool? isFollowing, int? followersCount}) => Profile(
        user: user,
        isOwn: isOwn,
        restricted: restricted,
        bio: bio,
        sports: sports,
        district: district,
        academyClub: academyClub,
        birthYear: birthYear,
        visibility: visibility,
        statsVisibility: statsVisibility,
        followersCount: followersCount ?? this.followersCount,
        followingCount: followingCount,
        isFollowing: isFollowing ?? this.isFollowing,
        isFollowedBy: isFollowedBy,
        isBlockedByMe: isBlockedByMe,
        canMessage: canMessage,
        badges: badges,
        coachingSinceYear: coachingSinceYear,
        consultationOffered: consultationOffered,
        organiser: organiser,
      );
}

class ProfileSportInput {
  const ProfileSportInput({required this.sportId, this.positionId, this.isPrimary = false});

  final int sportId;
  final int? positionId;
  final bool isPrimary;

  Map<String, dynamic> toJson() => {'sportId': sportId, 'positionId': positionId, 'isPrimary': isPrimary};
}

/// `ProfileUpdate`: every field optional; only non-null fields are sent.
class ProfileUpdate {
  const ProfileUpdate({
    this.displayName,
    this.bio,
    this.sports,
    this.city,
    this.district,
    this.state,
    this.academyClub,
    this.photoUploadId,
    this.visibility,
    this.statsVisibility,
    this.coachingSinceYear,
  });

  final String? displayName;
  final String? bio;
  final List<ProfileSportInput>? sports;
  final String? city;
  final String? district;
  final String? state;
  final String? academyClub;
  final int? photoUploadId;
  final String? visibility;
  final String? statsVisibility;
  final int? coachingSinceYear;

  Map<String, dynamic> toJson() => {
        if (displayName != null) 'displayName': displayName,
        if (bio != null) 'bio': bio,
        if (sports != null) 'sports': [for (final s in sports!) s.toJson()],
        if (city != null) 'city': city,
        if (district != null) 'district': district,
        if (state != null) 'state': state,
        if (academyClub != null) 'academyClub': academyClub,
        if (photoUploadId != null) 'photoUploadId': photoUploadId,
        if (visibility != null) 'visibility': visibility,
        if (statsVisibility != null) 'statsVisibility': statsVisibility,
        if (coachingSinceYear != null) 'coachingSinceYear': coachingSinceYear,
      };

  /// Never prints field values (the update holds personal details).
  @override
  String toString() => 'ProfileUpdate(${toJson().keys.join(', ')})';
}

/// Row of `GET /me/profile-viewers`. [viewedOn] is a date (no time).
class ProfileViewer {
  const ProfileViewer({required this.user, required this.viewedOn});

  final UserSummary user;
  final DateTime? viewedOn;

  factory ProfileViewer.fromJson(Map<String, dynamic> json) => ProfileViewer(
        user: UserSummary.fromJson(asMap(json['user'])),
        viewedOn: asDateTimeOrNull(json['viewedOn']),
      );
}

/// `StatEntry`. [stats] keys come from the template's fields.
class StatEntry {
  const StatEntry({
    required this.id,
    required this.sportId,
    required this.templateId,
    required this.season,
    required this.stats,
    required this.updatedAt,
  });

  final int id;
  final int sportId;
  final int templateId;
  final String season;
  final Map<String, dynamic> stats;
  final DateTime? updatedAt;

  factory StatEntry.fromJson(Map<String, dynamic> json) => StatEntry(
        id: asInt(json['id']),
        sportId: asInt(json['sportId']),
        templateId: asInt(json['templateId']),
        season: asString(json['season']),
        stats: asMap(json['stats']),
        updatedAt: asDateTimeOrNull(json['updatedAt']),
      );
}

/// `GET /profiles/:id/stats`. [totals] is null unless the owner has `advanced_stats`.
class StatsResult {
  const StatsResult({required this.entries, required this.totals});

  final List<StatEntry> entries;
  final Map<String, num>? totals;

  factory StatsResult.fromJson(Map<String, dynamic> json) {
    final t = asMapOrNull(json['totals']);
    return StatsResult(
      entries: asList(json['entries'], StatEntry.fromJson),
      totals: t == null
          ? null
          : {
              for (final e in t.entries)
                if (asNumOrNull(e.value) != null) e.key: asNumOrNull(e.value)!,
            },
    );
  }
}

/// Body of `POST /me/stats` (and `PUT /me/stats/:id`, which uses only season and stats).
class StatInput {
  const StatInput({required this.sportId, required this.templateId, required this.season, required this.stats});

  final int sportId;
  final int templateId;
  final String season;
  final Map<String, Object> stats;

  Map<String, dynamic> toCreateJson() => {'sportId': sportId, 'templateId': templateId, 'season': season, 'stats': stats};

  Map<String, dynamic> toUpdateJson() => {'season': season, 'stats': stats};
}

class Achievement {
  const Achievement({
    required this.id,
    required this.sportId,
    required this.title,
    required this.description,
    required this.level,
    required this.achievedOn,
    required this.certificateUrl,
    required this.createdAt,
  });

  final int id;
  final int? sportId;
  final String title;
  final String? description;
  final String level;
  final DateTime? achievedOn;

  /// Short-lived signed URL; open it, do not store it.
  final String? certificateUrl;
  final DateTime? createdAt;

  factory Achievement.fromJson(Map<String, dynamic> json) => Achievement(
        id: asInt(json['id']),
        sportId: asIntOrNull(json['sportId']),
        title: asString(json['title']),
        description: asStringOrNull(json['description']),
        level: asString(json['level'], 'other'),
        achievedOn: asDateTimeOrNull(json['achievedOn']),
        certificateUrl: asStringOrNull(json['certificateUrl']),
        createdAt: asDateTimeOrNull(json['createdAt']),
      );
}

class AchievementInput {
  const AchievementInput({
    required this.sportId,
    required this.title,
    required this.description,
    required this.level,
    required this.achievedOn,
    this.certificateUploadId,
  });

  final int? sportId;
  final String title;
  final String? description;
  final String level;
  final DateTime achievedOn;

  /// Only set when a new certificate was uploaded.
  final int? certificateUploadId;

  Map<String, dynamic> toJson() => {
        'sportId': sportId,
        'title': title,
        'description': description,
        'level': level,
        'achievedOn': formatDateOnly(achievedOn),
        if (certificateUploadId != null) 'certificateUploadId': certificateUploadId,
      };
}

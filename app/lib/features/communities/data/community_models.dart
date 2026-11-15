import '../../../core/json/json_utils.dart';
import '../../../core/models/sport.dart';
import '../../../core/models/user_summary.dart';

/// Wire values from docs/api.md "Communities".
abstract final class CommunityVisibility {
  static const public = 'public';
  static const private = 'private';
  static const values = [public, private];
}

abstract final class JoinPolicy {
  static const open = 'open';
  static const approval = 'approval';
  static const invite = 'invite';
  static const values = [open, approval, invite];
}

/// `Community.myStatus`.
abstract final class MembershipStatus {
  static const none = 'none';
  static const active = 'active';
  static const pending = 'pending';
  static const invited = 'invited';
  static const banned = 'banned';
}

/// `Community`. Its [id] is also the id of its chat conversation.
class Community {
  const Community({
    required this.id,
    required this.title,
    this.description,
    this.avatarUrl,
    this.sport,
    this.city,
    this.state,
    required this.visibility,
    required this.joinPolicy,
    required this.memberCount,
    this.owner,
    required this.myStatus,
    this.myRole,
    this.createdAt,
  });

  final int id;
  final String title;
  final String? description;
  final String? avatarUrl;
  final SportRef? sport;
  final String? city;
  final String? state;
  final String visibility;
  final String joinPolicy;
  final int memberCount;
  final UserSummary? owner;
  final String myStatus;

  /// owner | admin | member | null (not a member).
  final String? myRole;
  final DateTime? createdAt;

  bool get isMember => myStatus == MembershipStatus.active;
  bool get isOwner => isMember && myRole == 'owner';
  bool get canModerate => isMember && (myRole == 'owner' || myRole == 'admin');

  String get place => [city, state].where((s) => s != null && s.isNotEmpty).join(', ');

  factory Community.fromJson(Map<String, dynamic> json) => Community(
        id: asInt(json['id']),
        title: asString(json['title']),
        description: asStringOrNull(json['description']),
        avatarUrl: asStringOrNull(json['avatarUrl']),
        sport: SportRef.maybe(json['sport']),
        city: asStringOrNull(json['city']),
        state: asStringOrNull(json['state']),
        visibility: asString(json['visibility'], CommunityVisibility.public),
        joinPolicy: asString(json['joinPolicy'], JoinPolicy.open),
        memberCount: asInt(json['memberCount']),
        owner: UserSummary.maybe(json['owner']),
        myStatus: asString(json['myStatus'], MembershipStatus.none),
        myRole: asStringOrNull(json['myRole']),
        createdAt: asDateTimeOrNull(json['createdAt']),
      );
}

/// `CommunityInput`. For `PUT` send only changed fields ([toJson] with `only`).
class CommunityInput {
  const CommunityInput({
    required this.title,
    this.description,
    this.avatarUploadId,
    this.sportId,
    this.city,
    this.state,
    required this.visibility,
    required this.joinPolicy,
  });

  final String title;
  final String? description;

  /// From `MediaUploader.upload(MediaPurpose.communityAvatar, …)`; null keeps the current avatar.
  final int? avatarUploadId;
  final int? sportId;
  final String? city;
  final String? state;
  final String visibility;
  final String joinPolicy;

  Map<String, dynamic> toJson({Set<String>? only}) {
    final all = <String, dynamic>{
      'title': title,
      'description': description,
      'avatarUploadId': avatarUploadId,
      'sportId': sportId,
      'city': city,
      'state': state,
      'visibility': visibility,
      'joinPolicy': joinPolicy,
    };
    if (only == null) return all;
    return {for (final e in all.entries) if (only.contains(e.key)) e.key: e.value};
  }
}

/// Item of `GET /communities/:id/join-requests`.
class JoinRequest {
  const JoinRequest({required this.user, this.requestedAt});

  final UserSummary user;
  final DateTime? requestedAt;

  factory JoinRequest.fromJson(Map<String, dynamic> json) => JoinRequest(
        user: UserSummary.fromJson(asMap(json['user'])),
        requestedAt: asDateTimeOrNull(json['requestedAt']),
      );
}

/// Response of `POST /communities/:id/invites`.
class InviteResult {
  const InviteResult({required this.invited, required this.skipped});

  final List<int> invited;
  final List<int> skipped;

  factory InviteResult.fromJson(Map<String, dynamic> json) =>
      InviteResult(invited: asIntList(json['invited']), skipped: asIntList(json['skipped']));
}

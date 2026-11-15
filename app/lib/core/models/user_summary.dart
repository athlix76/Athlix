import '../json/json_utils.dart';
import 'sport.dart';

/// `UserSummary`: the only shape used to show another user.
/// `photoUrl` is a short-lived signed URL; never cache it across sessions.
class UserSummary {
  const UserSummary({
    required this.id,
    required this.displayName,
    required this.photoUrl,
    required this.primaryRole,
    required this.sport,
    required this.position,
    required this.city,
    required this.state,
    required this.isVerified,
  });

  final int id;
  final String displayName;
  final String? photoUrl;
  final String? primaryRole;
  final SportRef? sport;
  final SportRef? position;
  final String? city;
  final String? state;
  final bool isVerified;

  factory UserSummary.fromJson(Map<String, dynamic> json) => UserSummary(
        id: asInt(json['id']),
        displayName: asString(json['displayName']),
        photoUrl: asStringOrNull(json['photoUrl']),
        primaryRole: asStringOrNull(json['primaryRole']),
        sport: SportRef.maybe(json['sport']),
        position: SportRef.maybe(json['position']),
        city: asStringOrNull(json['city']),
        state: asStringOrNull(json['state']),
        isVerified: asBool(json['isVerified']),
      );

  static UserSummary? maybe(Object? json) {
    final m = asMapOrNull(json);
    return m == null ? null : UserSummary.fromJson(m);
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'displayName': displayName,
        'photoUrl': photoUrl,
        'primaryRole': primaryRole,
        'sport': sport?.toJson(),
        'position': position?.toJson(),
        'city': city,
        'state': state,
        'isVerified': isVerified,
      };

  /// "Cricket · Pune" style subtitle for lists.
  String get subtitle {
    final parts = <String>[
      if (sport != null) sport!.name,
      if (position != null) position!.name,
      if (city != null && city!.isNotEmpty) city!,
    ];
    return parts.join(' · ');
  }
}

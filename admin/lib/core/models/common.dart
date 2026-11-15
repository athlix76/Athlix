import '../api/json.dart';

/// `SportRef` from docs/api.md.
class SportRef {
  const SportRef({required this.id, required this.code, required this.name});

  final int id;
  final String code;
  final String name;

  static SportRef? maybe(Object? v) {
    final j = asJsonOrNull(v);
    if (j == null) return null;
    return SportRef(id: asInt(j['id']), code: asString(j['code']), name: asString(j['name']));
  }
}

/// `UserSummary` from docs/api.md: the only shape used to show another user.
class UserSummary {
  const UserSummary({
    required this.id,
    required this.displayName,
    this.photoUrl,
    this.primaryRole,
    this.sport,
    this.city,
    this.state,
    this.isVerified = false,
  });

  final int id;
  final String displayName;
  final String? photoUrl;
  final String? primaryRole;
  final SportRef? sport;
  final String? city;
  final String? state;
  final bool isVerified;

  factory UserSummary.fromJson(Json j) => UserSummary(
        id: asInt(j['id']),
        displayName: asString(j['displayName'], 'User #${asInt(j['id'])}'),
        photoUrl: asStringOrNull(j['photoUrl']),
        primaryRole: asStringOrNull(j['primaryRole']),
        sport: SportRef.maybe(j['sport']),
        city: asStringOrNull(j['city']),
        state: asStringOrNull(j['state']),
        isVerified: asBool(j['isVerified']),
      );

  static UserSummary? maybe(Object? v) {
    final j = asJsonOrNull(v);
    return j == null ? null : UserSummary.fromJson(j);
  }

  String get label => '$displayName (#$id)';
}

/// `{id, name}` reference to an admin (audit log, moderation actions, config history).
class AdminRef {
  const AdminRef(this.id, this.name);

  final int id;
  final String name;

  static AdminRef? maybe(Object? v) {
    final j = asJsonOrNull(v);
    return j == null ? null : AdminRef(asInt(j['id']), asString(j['name']));
  }
}

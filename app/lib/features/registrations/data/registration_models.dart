import '../../../core/json/json_utils.dart';
import '../../../core/models/user_summary.dart';
import '../../events/data/event_models.dart';
import '../../location_identity/data/geo_models.dart';

abstract final class RegistrationEnums {
  static const statuses = ['registered', 'waitlisted', 'withdrawn', 'rejected'];
  static const paymentStatuses = ['unpaid', 'paid_online', 'paid_offline', 'waived'];

  /// Contract cap from docs/api.md ("Max 30 team members"); not in public config.
  static const maxTeamMembers = 30;

  static const eligibilityFlags = [
    'none',
    'identity_expired',
    'identity_rejected',
    'identity_revoked',
    'identity_changed',
  ];
}

class TeamMember {
  const TeamMember({required this.name, this.role});

  final String name;
  final String? role;

  factory TeamMember.fromJson(Map<String, dynamic> json) =>
      TeamMember(name: asString(json['name']), role: asStringOrNull(json['role']));

  Map<String, dynamic> toJson() => {'name': name, if (role != null && role!.isNotEmpty) 'role': role};
}

/// `Registration`; also `RegistrationForOrganiser` when [user] is set.
class Registration {
  const Registration({
    required this.id,
    required this.eventId,
    required this.status,
    required this.paymentStatus,
    this.paymentNote,
    this.teamName,
    required this.teamMembers,
    this.createdAt,
    this.event,
    this.user,
    this.qualifiedVia,
    this.qualifiedDistrict,
    this.eligibilityFlag = 'none',
    this.eligibilityFlaggedAt,
    this.qualifiedLabel,
  });

  final int id;
  final int eventId;
  final String status;
  final String paymentStatus;
  final String? paymentNote;
  final String? teamName;
  final List<TeamMember> teamMembers;
  final DateTime? createdAt;
  final EventSummary? event;
  final UserSummary? user;

  /// `open|native|residence`: the identity the player registered with.
  final String? qualifiedVia;
  final District? qualifiedDistrict;

  /// Set when that identity stopped being valid. Never cancels the
  /// registration; the organiser decides.
  final String eligibilityFlag;
  final DateTime? eligibilityFlaggedAt;

  /// Organiser view only: "Native: Thanjavur (verified)". Never a document.
  final String? qualifiedLabel;

  bool get isFlagged => eligibilityFlag != 'none';

  bool get isActive => status == 'registered' || status == 'waitlisted';

  factory Registration.fromJson(Map<String, dynamic> json) {
    final ev = asMapOrNull(json['event']);
    return Registration(
      id: asInt(json['id']),
      eventId: asInt(json['eventId']),
      status: asString(json['status']),
      paymentStatus: asString(json['paymentStatus'], 'unpaid'),
      paymentNote: asStringOrNull(json['paymentNote']),
      teamName: asStringOrNull(json['teamName']),
      teamMembers: asList(json['teamMembers'], TeamMember.fromJson),
      createdAt: asDateTimeOrNull(json['createdAt']),
      event: ev == null ? null : EventSummary.fromJson(ev),
      user: UserSummary.maybe(json['user']),
      qualifiedVia: asStringOrNull(json['qualifiedVia']),
      qualifiedDistrict: District.maybe(json['qualifiedDistrict']),
      eligibilityFlag: asString(json['eligibilityFlag'], 'none'),
      eligibilityFlaggedAt: asDateTimeOrNull(json['eligibilityFlaggedAt']),
      qualifiedLabel: asStringOrNull(json['qualifiedLabel']),
    );
  }
}

/// Banner text for a flagged registration (player view).
String eligibilityFlagText(String flag) => switch (flag) {
      'identity_expired' => 'Your residence proof expired after you registered. The organiser will review your registration.',
      'identity_rejected' => 'The location proof you registered with was rejected. The organiser will review your registration.',
      'identity_revoked' => 'The location identity you registered with was revoked. The organiser will review your registration.',
      _ => 'Your location identity changed; the organiser will review your registration.',
    };

import '../../../core/json/json_utils.dart';
import '../../../core/models/user_summary.dart';

/// Consultation modes.
abstract final class ConsultationMode {
  static const online = 'online';
  static const inPerson = 'in_person';

  static const values = [online, inPerson];
  static const labels = {online: 'Online', inPerson: 'In person'};
}

/// Request statuses.
abstract final class ConsultationStatus {
  static const pending = 'pending';
  static const accepted = 'accepted';
  static const declined = 'declined';
  static const cancelled = 'cancelled';
  static const completed = 'completed';

  static const values = [pending, accepted, declined, cancelled, completed];
}

/// `?as=` of `GET /me/consultation-requests`.
abstract final class ConsultationSide {
  static const player = 'player';
  static const coach = 'coach';
}

/// `GET /coaches/:userId/consultation` and `PUT /me/consultation`.
/// Athlix only lists the offer: the fee (`feeNote`) is paid directly to the coach.
class ConsultationOffer {
  const ConsultationOffer({
    required this.coach,
    required this.offered,
    required this.description,
    required this.feeNote,
    required this.modes,
    required this.canRequest,
  });

  final UserSummary? coach;
  final bool offered;
  final String? description;
  final String? feeNote;
  final List<String> modes;
  final bool canRequest;

  factory ConsultationOffer.fromJson(Map<String, dynamic> json) => ConsultationOffer(
        coach: UserSummary.maybe(json['coach']),
        offered: asBool(json['offered']),
        description: asStringOrNull(json['description']),
        feeNote: asStringOrNull(json['feeNote']),
        modes: asStringList(json['modes']),
        canRequest: asBool(json['canRequest']),
      );
}

/// Body of `PUT /me/consultation`.
class ConsultationOfferInput {
  const ConsultationOfferInput({
    required this.offered,
    this.description,
    this.feeNote,
    this.modes = const [],
  });

  final bool offered;
  final String? description;
  final String? feeNote;
  final List<String> modes;

  Map<String, dynamic> toJson() => {
        'offered': offered,
        'description': description,
        'feeNote': feeNote,
        'modes': modes,
      };
}

/// `ConsultationRequest`.
class ConsultationRequest {
  const ConsultationRequest({
    required this.id,
    required this.coach,
    required this.player,
    required this.message,
    required this.preferredMode,
    required this.status,
    required this.createdAt,
    required this.updatedAt,
  });

  final int id;
  final UserSummary coach;
  final UserSummary player;
  final String message;
  final String preferredMode;
  final String status;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  bool get isPending => status == ConsultationStatus.pending;
  bool get isAccepted => status == ConsultationStatus.accepted;

  factory ConsultationRequest.fromJson(Map<String, dynamic> json) => ConsultationRequest(
        id: asInt(json['id']),
        coach: UserSummary.fromJson(asMap(json['coach'])),
        player: UserSummary.fromJson(asMap(json['player'])),
        message: asString(json['message']),
        preferredMode: asString(json['preferredMode'], ConsultationMode.online),
        status: asString(json['status'], ConsultationStatus.pending),
        createdAt: asDateTimeOrNull(json['createdAt']),
        updatedAt: asDateTimeOrNull(json['updatedAt']),
      );
}

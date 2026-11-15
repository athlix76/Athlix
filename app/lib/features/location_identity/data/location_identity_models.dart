import '../../../core/json/json_utils.dart';
import 'geo_models.dart';

abstract final class IdentityType {
  static const native = 'native';
  static const residence = 'residence';
}

abstract final class DisplayStatus {
  static const notSubmitted = 'not_submitted';
  static const pending = 'pending';
  static const verified = 'verified';
  static const rejected = 'rejected';
  static const expired = 'expired';
}

/// `LocationIdentity`. Holds no document URL: proofs are never shown back.
class LocationIdentity {
  const LocationIdentity({
    required this.id,
    required this.identityType,
    this.district,
    this.state,
    required this.source,
    required this.proofType,
    required this.proofHolder,
    this.aadhaarLast4,
    required this.status,
    this.rejectionCode,
    this.rejectionReason,
    this.submittedAt,
    this.reviewedAt,
    this.verifiedAt,
    this.expiresAt,
  });

  final int id;
  final String identityType;
  final District? district;
  final GeoState? state;
  final String source;
  final String proofType;
  final String proofHolder;
  final String? aadhaarLast4;
  final String status;
  final String? rejectionCode;
  final String? rejectionReason;
  final DateTime? submittedAt;
  final DateTime? reviewedAt;
  final DateTime? verifiedAt;
  final DateTime? expiresAt;

  bool get fromDigilocker => source == 'digilocker';

  /// Only `pending` and `verified` rows can be withdrawn (`DELETE`).
  bool get canWithdraw => status == 'pending' || status == 'verified';

  String get place => [district?.name, state?.name].whereType<String>().join(', ');

  factory LocationIdentity.fromJson(Map<String, dynamic> json) => LocationIdentity(
        id: asInt(json['id']),
        identityType: asString(json['identityType']),
        district: District.maybe(json['district']),
        state: GeoState.maybe(json['state']),
        source: asString(json['source'], 'upload'),
        proofType: asString(json['proofType']),
        proofHolder: asString(json['proofHolder'], 'self'),
        aadhaarLast4: asStringOrNull(json['aadhaarLast4']),
        status: asString(json['status']),
        rejectionCode: asStringOrNull(json['rejectionCode']),
        rejectionReason: asStringOrNull(json['rejectionReason']),
        submittedAt: asDateTimeOrNull(json['submittedAt']),
        reviewedAt: asDateTimeOrNull(json['reviewedAt']),
        verifiedAt: asDateTimeOrNull(json['verifiedAt']),
        expiresAt: asDateTimeOrNull(json['expiresAt']),
      );

  static LocationIdentity? maybe(Object? v) {
    final m = asMapOrNull(v);
    return m == null ? null : LocationIdentity.fromJson(m);
  }

  @override
  String toString() => 'LocationIdentity($id, $identityType, $status)';
}

/// `IdentitySlot`: one per identity type.
class IdentitySlot {
  const IdentitySlot({
    required this.displayStatus,
    this.current,
    this.pending,
    required this.canSubmit,
    this.nextChangeAt,
    this.expiresInDays,
  });

  final String displayStatus;
  final LocationIdentity? current;
  final LocationIdentity? pending;
  final bool canSubmit;
  final DateTime? nextChangeAt;
  final int? expiresInDays;

  static const empty = IdentitySlot(displayStatus: DisplayStatus.notSubmitted, canSubmit: true);

  factory IdentitySlot.fromJson(Map<String, dynamic>? json) {
    if (json == null) return empty;
    return IdentitySlot(
      displayStatus: asString(json['displayStatus'], DisplayStatus.notSubmitted),
      current: LocationIdentity.maybe(json['current']),
      pending: LocationIdentity.maybe(json['pending']),
      canSubmit: asBool(json['canSubmit'], true),
      nextChangeAt: asDateTimeOrNull(json['nextChangeAt']),
      expiresInDays: asIntOrNull(json['expiresInDays']),
    );
  }
}

/// `LocationIdentities.notice`: the privacy notice the user must accept.
class LocationNotice {
  const LocationNotice({required this.policyDocumentId, required this.version, required this.title});

  final int policyDocumentId;
  final int version;
  final String title;

  static LocationNotice? maybe(Object? v) {
    final m = asMapOrNull(v);
    if (m == null) return null;
    return LocationNotice(
      policyDocumentId: asInt(m['policyDocumentId']),
      version: asInt(m['version'], 1),
      title: asString(m['title']),
    );
  }
}

/// `GET /me/location-identities`.
class LocationIdentities {
  const LocationIdentities({
    required this.native,
    required this.residence,
    this.notice,
    required this.nativeProofTypes,
    required this.residenceProofTypes,
    required this.guardianProofAllowed,
  });

  final IdentitySlot native;
  final IdentitySlot residence;
  final LocationNotice? notice;
  final List<String> nativeProofTypes;
  final List<String> residenceProofTypes;
  final bool guardianProofAllowed;

  IdentitySlot slot(String type) => type == IdentityType.native ? native : residence;

  List<String> proofTypesFor(String type) =>
      type == IdentityType.native ? nativeProofTypes : residenceProofTypes;

  factory LocationIdentities.fromJson(Map<String, dynamic> json) {
    final proof = asMapOrNull(json['proofTypes']);
    return LocationIdentities(
      native: IdentitySlot.fromJson(asMapOrNull(json['native'])),
      residence: IdentitySlot.fromJson(asMapOrNull(json['residence'])),
      notice: LocationNotice.maybe(json['notice']),
      nativeProofTypes: asStringList(proof?['native']),
      residenceProofTypes: asStringList(proof?['residence']),
      guardianProofAllowed: asBool(json['guardianProofAllowed']),
    );
  }
}

/// Body of `POST /me/location-identities`. The state is derived by the server.
class LocationIdentityInput {
  const LocationIdentityInput({
    required this.identityType,
    required this.districtId,
    required this.proofType,
    required this.proofUploadId,
    required this.proofHolder,
    required this.noticePolicyDocumentId,
  });

  final String identityType;
  final int districtId;
  final String proofType;
  final int proofUploadId;
  final String proofHolder;
  final int noticePolicyDocumentId;

  Map<String, dynamic> toJson() => {
        'identityType': identityType,
        'districtId': districtId,
        'proofType': proofType,
        'proofUploadId': proofUploadId,
        'proofHolder': proofHolder,
        'noticePolicyDocumentId': noticePolicyDocumentId,
      };
}

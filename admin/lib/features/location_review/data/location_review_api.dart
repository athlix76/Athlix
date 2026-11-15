import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';
import '../../../core/models/common.dart';

const identityStatuses = ['pending', 'verified', 'rejected', 'expired', 'superseded', 'revoked', 'withdrawn'];
const identityTypes = ['native', 'residence'];

/// Proof types that carry a dated document the checklist asks the reviewer to date-check.
const datedProofTypes = {'utility_bill', 'bank_statement'};
const maskedAadhaarProof = 'masked_aadhaar';

/// `{id, name}` of a District or State (the other fields are not needed here).
class PlaceRef {
  const PlaceRef(this.id, this.name);

  final int id;
  final String name;

  static PlaceRef? maybe(Object? v) {
    final j = asJsonOrNull(v);
    return j == null ? null : PlaceRef(asInt(j['id']), asString(j['name']));
  }
}

/// `LocationIdentity` (history rows) and the shared part of the admin rows.
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
  final PlaceRef? district;
  final PlaceRef? state;
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

  String get placeLabel => [district?.name, state?.name].whereType<String>().join(', ');

  factory LocationIdentity.fromJson(Json j) => LocationIdentity(
        id: asInt(j['id']),
        identityType: asString(j['identityType']),
        district: PlaceRef.maybe(j['district']),
        state: PlaceRef.maybe(j['state']),
        source: asString(j['source'], 'upload'),
        proofType: asString(j['proofType']),
        proofHolder: asString(j['proofHolder'], 'self'),
        aadhaarLast4: asStringOrNull(j['aadhaarLast4']),
        status: asString(j['status']),
        rejectionCode: asStringOrNull(j['rejectionCode']),
        rejectionReason: asStringOrNull(j['rejectionReason']),
        submittedAt: asDate(j['submittedAt']),
        reviewedAt: asDate(j['reviewedAt']),
        verifiedAt: asDate(j['verifiedAt']),
        expiresAt: asDate(j['expiresAt']),
      );
}

/// `AdminLocationRow`.
class LocationRow {
  const LocationRow({required this.identity, required this.user, required this.isMinor});

  final LocationIdentity identity;
  final UserSummary user;
  final bool isMinor;

  factory LocationRow.fromJson(Json j) =>
      LocationRow(identity: LocationIdentity.fromJson(j), user: UserSummary.fromJson(asJson(j['user'])), isMinor: asBool(j['isMinor']));
}

/// `AdminLocationDetail`. [proofUrl] is a short-lived signed URL: it is kept
/// only in this object (screen memory) and never stored or downloaded.
class LocationDetail {
  const LocationDetail({
    required this.row,
    this.proofUrl,
    this.proofContentType,
    this.proofPurgedAt,
    this.reviewedBy,
    this.reviewNote,
    this.profileDistrict,
    required this.history,
    this.utilityBillMaxAgeDays,
    required this.rejectionReasons,
    required this.fetchedAt,
  });

  final LocationRow row;
  final String? proofUrl;
  final String? proofContentType;
  final DateTime? proofPurgedAt;
  final AdminRef? reviewedBy;

  /// Internal approve note (admin views only, never shown to the user).
  final String? reviewNote;

  /// Free text from the user's profile: a hint only, never proof.
  final String? profileDistrict;
  final List<LocationIdentity> history;
  final int? utilityBillMaxAgeDays;
  final List<String> rejectionReasons;
  final DateTime fetchedAt;

  LocationIdentity get identity => row.identity;

  bool get isPdf => proofContentType == 'application/pdf';

  factory LocationDetail.fromJson(Json j, {DateTime? now}) {
    final checklist = asJson(j['checklist']);
    return LocationDetail(
      row: LocationRow.fromJson(j),
      proofUrl: asStringOrNull(j['proofUrl']),
      proofContentType: asStringOrNull(j['proofContentType']),
      proofPurgedAt: asDate(j['proofPurgedAt']),
      reviewedBy: AdminRef.maybe(j['reviewedBy']),
      reviewNote: asStringOrNull(j['reviewNote']),
      profileDistrict: asStringOrNull(j['profileDistrict']),
      history: asList(j['history'], LocationIdentity.fromJson),
      utilityBillMaxAgeDays: asIntOrNull(checklist['utilityBillMaxAgeDays']),
      rejectionReasons: asStringList(checklist['rejectionReasons']),
      fetchedAt: now ?? DateTime.now(),
    );
  }
}

class LocationQueueFilters {
  const LocationQueueFilters({this.status, this.identityType, this.stateId, this.districtId});

  final String? status;
  final String? identityType;
  final int? stateId;
  final int? districtId;

  Map<String, Object?> toQuery() => {'status': status, 'identityType': identityType, 'stateId': stateId, 'districtId': districtId};
}

class LocationReviewApi {
  LocationReviewApi(this._client);

  final ApiClient _client;

  /// `GET /admin/location-identities` (location.review). Pending: oldest first.
  Future<PageResult<LocationRow>> queue(LocationQueueFilters f, String? cursor) =>
      _client.page('/admin/location-identities', LocationRow.fromJson, query: f.toQuery(), cursor: cursor);

  /// `GET /admin/location-identities/:id` (location.review). Issues a fresh
  /// signed proof URL; the backend audits every call (`location.proof_viewed`).
  Future<LocationDetail> detail(int id) async => LocationDetail.fromJson(asJson(await _client.get('/admin/location-identities/$id')));

  /// `POST /admin/location-identities/:id/approve`. `aadhaarLast4` only for masked Aadhaar;
  /// `note` (at most 255) is stored and returned as [LocationDetail.reviewNote].
  Future<LocationDetail> approve(int id, {String? aadhaarLast4, String? note}) async => LocationDetail.fromJson(asJson(await _client.post(
        '/admin/location-identities/$id/approve',
        body: {if (aadhaarLast4 != null) 'aadhaarLast4': aadhaarLast4, if (note != null) 'note': note},
      )));

  /// `POST /admin/location-identities/:id/reject`.
  Future<LocationDetail> reject(int id, {required String rejectionCode, String? rejectionReason}) async =>
      LocationDetail.fromJson(asJson(await _client.post(
        '/admin/location-identities/$id/reject',
        body: {'rejectionCode': rejectionCode, if (rejectionReason != null) 'rejectionReason': rejectionReason},
      )));

  /// `POST /admin/location-identities/:id/revoke` (verified → revoked).
  Future<LocationDetail> revoke(int id, String reason) async =>
      LocationDetail.fromJson(asJson(await _client.post('/admin/location-identities/$id/revoke', body: {'reason': reason})));
}

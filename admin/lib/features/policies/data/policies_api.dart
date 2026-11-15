import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';

/// Policy document types (api.md PolicyDocument + data-model.md).
const policyDocTypes = [
  'terms',
  'privacy',
  'listing_refund',
  'guardian_consent',
  'community_guidelines',
  'organiser_terms',
  'entry_fee_disclaimer',
  'location_identity_notice',
];

/// `PolicyDocument + {active, createdAt}`.
class PolicyDocument {
  const PolicyDocument({
    required this.id,
    required this.docType,
    required this.version,
    required this.title,
    required this.contentMd,
    required this.requiresAcceptance,
    this.effectiveFrom,
    required this.active,
    this.createdAt,
  });

  final int id;
  final String docType;
  final int version;
  final String title;
  final String contentMd;
  final bool requiresAcceptance;
  final DateTime? effectiveFrom;
  final bool active;
  final DateTime? createdAt;

  factory PolicyDocument.fromJson(Json j) => PolicyDocument(
        id: asInt(j['id']),
        docType: asString(j['docType']),
        version: asInt(j['version']),
        title: asString(j['title']),
        contentMd: asString(j['contentMd']),
        requiresAcceptance: asBool(j['requiresAcceptance']),
        effectiveFrom: asDate(j['effectiveFrom']),
        active: asBool(j['active']),
        createdAt: asDate(j['createdAt']),
      );
}

class PoliciesApi {
  PoliciesApi(this._client);

  final ApiClient _client;

  /// `GET /admin/policies` (config.read).
  Future<List<PolicyDocument>> list({String? docType}) async =>
      asList(await _client.get('/admin/policies', query: {'docType': docType}), PolicyDocument.fromJson);

  /// `POST /admin/policies` (policies.write): a new, inactive version.
  Future<PolicyDocument> create({required String docType, required String title, required String contentMd, required bool requiresAcceptance}) async =>
      PolicyDocument.fromJson(asJson(await _client.post('/admin/policies', body: {
        'docType': docType,
        'title': title,
        'contentMd': contentMd,
        'requiresAcceptance': requiresAcceptance,
      })));

  /// `POST /admin/policies/:id/activate` (policies.write).
  Future<PolicyDocument> activate(int id) async => PolicyDocument.fromJson(asJson(await _client.post('/admin/policies/$id/activate')));
}

import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';

const requirementRoles = ['player', 'coach', 'organiser'];
const requirementKinds = ['digilocker', 'guardian_consent_if_minor', 'organiser_approval', 'coach_pro_subscription'];
const requirementGates = ['app_access', 'create_event', 'offer_consultation', 'create_community'];

/// `{id, role, requirement, isRequired, gates}`.
class VerificationRequirement {
  const VerificationRequirement({required this.id, required this.role, required this.requirement, required this.isRequired, required this.gates});

  final int id;
  final String role;
  final String requirement;
  final bool isRequired;
  final List<String> gates;

  factory VerificationRequirement.fromJson(Json j) => VerificationRequirement(
        id: asInt(j['id']),
        role: asString(j['role']),
        requirement: asString(j['requirement']),
        isRequired: asBool(j['isRequired']),
        gates: asStringList(j['gates']),
      );
}

class VerificationRequirementsApi {
  VerificationRequirementsApi(this._client);

  final ApiClient _client;

  /// `GET /admin/verification-requirements` (config.read).
  Future<List<VerificationRequirement>> list() async =>
      asList(await _client.get('/admin/verification-requirements'), VerificationRequirement.fromJson);

  /// `POST /admin/verification-requirements` (config.write).
  Future<VerificationRequirement> create({required String role, required String requirement, required bool isRequired, required List<String> gates}) async =>
      VerificationRequirement.fromJson(asJson(await _client.post('/admin/verification-requirements', body: {
        'role': role,
        'requirement': requirement,
        'isRequired': isRequired,
        'gates': gates,
      })));

  /// `PUT /admin/verification-requirements/:id` (config.write). Only `isRequired` can change.
  Future<VerificationRequirement> setRequired(int id, bool isRequired) async => VerificationRequirement.fromJson(
      asJson(await _client.put('/admin/verification-requirements/$id', body: {'isRequired': isRequired})));
}

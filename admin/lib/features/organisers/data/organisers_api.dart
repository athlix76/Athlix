import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';
import '../../../core/models/common.dart';

class OrganiserApplicationRow {
  const OrganiserApplicationRow({required this.user, this.orgName, this.orgType, required this.status, this.submittedAt});

  final UserSummary user;
  final String? orgName;
  final String? orgType;
  final String status;
  final DateTime? submittedAt;

  factory OrganiserApplicationRow.fromJson(Json j) => OrganiserApplicationRow(
        user: UserSummary.fromJson(asJson(j['user'])),
        orgName: asStringOrNull(j['orgName']),
        orgType: asStringOrNull(j['orgType']),
        status: asString(j['status']),
        submittedAt: asDate(j['submittedAt']),
      );
}

class OrganiserStepSubmission {
  const OrganiserStepSubmission({
    required this.stepId,
    required this.stepCode,
    required this.title,
    required this.description,
    required this.inputType,
    required this.isRequired,
    this.valueText,
    this.documentUrl,
    this.checked,
    this.status,
    this.reviewNote,
    required this.submitted,
  });

  final int stepId;
  final String stepCode;
  final String title;
  final String description;
  final String inputType;
  final bool isRequired;
  final String? valueText;

  /// Signed, short-lived. Opened in a new tab, never stored.
  final String? documentUrl;
  final bool? checked;
  final String? status;
  final String? reviewNote;
  final bool submitted;

  factory OrganiserStepSubmission.fromJson(Json j) {
    final s = asJsonOrNull(j['submission']);
    final doc = s == null ? null : asJsonOrNull(s['document']);
    return OrganiserStepSubmission(
      stepId: asInt(j['stepId']),
      stepCode: asString(j['stepCode']),
      title: asString(j['title']),
      description: asString(j['description']),
      inputType: asString(j['inputType']),
      isRequired: asBool(j['isRequired']),
      valueText: s == null ? null : asStringOrNull(s['valueText']),
      documentUrl: doc == null ? null : asStringOrNull(doc['url']),
      checked: s == null || s['checked'] == null ? null : asBool(s['checked']),
      status: s == null ? null : asStringOrNull(s['status']),
      reviewNote: s == null ? null : asStringOrNull(s['reviewNote']),
      submitted: s != null,
    );
  }
}

class OrganiserApplication {
  const OrganiserApplication({
    required this.user,
    required this.status,
    this.orgName,
    this.orgType,
    this.reviewNote,
    this.submittedAt,
    this.reviewedAt,
    required this.steps,
  });

  final UserSummary user;
  final String status;
  final String? orgName;
  final String? orgType;
  final String? reviewNote;
  final DateTime? submittedAt;
  final DateTime? reviewedAt;
  final List<OrganiserStepSubmission> steps;

  factory OrganiserApplication.fromJson(Json j) => OrganiserApplication(
        user: UserSummary.fromJson(asJson(j['user'])),
        status: asString(j['status']),
        orgName: asStringOrNull(j['orgName']),
        orgType: asStringOrNull(j['orgType']),
        reviewNote: asStringOrNull(j['reviewNote']),
        submittedAt: asDate(j['submittedAt']),
        reviewedAt: asDate(j['reviewedAt']),
        steps: asList(j['steps'], OrganiserStepSubmission.fromJson),
      );
}

class OrganisersApi {
  OrganisersApi(this._client);

  final ApiClient _client;

  /// `GET /admin/organisers` (organisers.review).
  Future<PageResult<OrganiserApplicationRow>> list(String? status, String? cursor) =>
      _client.page('/admin/organisers', OrganiserApplicationRow.fromJson, query: {'status': status}, cursor: cursor);

  /// `GET /admin/organisers/:userId` (organisers.review).
  Future<OrganiserApplication> detail(int userId) async =>
      OrganiserApplication.fromJson(asJson(await _client.get('/admin/organisers/$userId')));

  /// `POST /admin/organisers/:userId/approve` (organisers.review).
  Future<OrganiserApplication> approve(int userId, String? note) async => OrganiserApplication.fromJson(
      asJson(await _client.post('/admin/organisers/$userId/approve', body: {if (note != null) 'note': note})));

  /// `POST /admin/organisers/:userId/reject` (organisers.review).
  Future<OrganiserApplication> reject(int userId, String note) async =>
      OrganiserApplication.fromJson(asJson(await _client.post('/admin/organisers/$userId/reject', body: {'note': note})));
}

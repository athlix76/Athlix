import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';
import '../../../core/models/common.dart';

const reportStatuses = ['open', 'actioned', 'dismissed'];
const reportTargetTypes = ['user', 'event', 'post', 'comment', 'message', 'conversation'];
const reportReasons = ['spam', 'fake_profile', 'harassment', 'scam', 'inappropriate', 'underage_safety', 'payment_issue', 'other'];
const reportResolveActions = ['dismiss', 'hide', 'remove', 'suspend_user', 'warn'];

/// `AdminReport`.
class AdminReport {
  const AdminReport({
    required this.id,
    this.reporter,
    required this.targetType,
    required this.targetId,
    this.previewTitle,
    this.previewText,
    this.ownerId,
    required this.reason,
    this.details,
    required this.status,
    required this.openReportsOnTarget,
    this.resolutionNote,
    this.createdAt,
    this.resolvedAt,
  });

  final int id;
  final UserSummary? reporter;
  final String targetType;
  final int targetId;
  final String? previewTitle;
  final String? previewText;
  final int? ownerId;
  final String reason;
  final String? details;
  final String status;
  final int openReportsOnTarget;
  final String? resolutionNote;
  final DateTime? createdAt;
  final DateTime? resolvedAt;

  factory AdminReport.fromJson(Json j) {
    final p = asJson(j['targetPreview']);
    return AdminReport(
      id: asInt(j['id']),
      reporter: UserSummary.maybe(j['reporter']),
      targetType: asString(j['targetType']),
      targetId: asInt(j['targetId']),
      previewTitle: asStringOrNull(p['title']),
      previewText: asStringOrNull(p['text']),
      ownerId: asIntOrNull(p['ownerId']),
      reason: asString(j['reason']),
      details: asStringOrNull(j['details']),
      status: asString(j['status']),
      openReportsOnTarget: asInt(j['openReportsOnTarget']),
      resolutionNote: asStringOrNull(j['resolutionNote']),
      createdAt: asDate(j['createdAt']),
      resolvedAt: asDate(j['resolvedAt']),
    );
  }
}

class ReportFilters {
  const ReportFilters({this.status, this.targetType, this.reason});

  final String? status;
  final String? targetType;
  final String? reason;
}

class ReportsApi {
  ReportsApi(this._client);

  final ApiClient _client;

  /// `GET /admin/reports` (reports.read).
  Future<PageResult<AdminReport>> list(ReportFilters f, String? cursor) => _client.page(
        '/admin/reports',
        AdminReport.fromJson,
        query: {'status': f.status, 'targetType': f.targetType, 'reason': f.reason},
        cursor: cursor,
      );

  /// `POST /admin/reports/:id/resolve` (reports.resolve).
  Future<AdminReport> resolve(int id, String action, String? note) async =>
      AdminReport.fromJson(asJson(await _client.post('/admin/reports/$id/resolve', body: {'action': action, 'note': note})));
}

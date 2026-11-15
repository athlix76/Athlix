import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';
import '../../../core/models/common.dart';

class AuditEntry {
  const AuditEntry({
    required this.id,
    this.admin,
    required this.action,
    required this.targetType,
    this.targetId,
    this.details,
    this.createdAt,
  });

  final int id;
  final AdminRef? admin;
  final String action;
  final String targetType;
  final String? targetId;
  final Object? details;
  final DateTime? createdAt;

  factory AuditEntry.fromJson(Json j) => AuditEntry(
        id: asInt(j['id']),
        admin: AdminRef.maybe(j['admin']),
        action: asString(j['action']),
        targetType: asString(j['targetType']),
        // Config keys are strings, most other targets are numeric ids.
        targetId: asStringOrNull(j['targetId']),
        details: j['details'],
        createdAt: asDate(j['createdAt']),
      );
}

class AuditFilters {
  const AuditFilters({this.adminId, this.action, this.targetType, this.targetId});

  final int? adminId;
  final String? action;
  final String? targetType;
  final String? targetId;
}

class AuditApi {
  AuditApi(this._client);

  final ApiClient _client;

  /// `GET /admin/audit-log` (audit.read).
  Future<PageResult<AuditEntry>> list(AuditFilters f, {String? cursor}) => _client.page(
        '/admin/audit-log',
        AuditEntry.fromJson,
        query: {'adminId': f.adminId, 'action': f.action, 'targetType': f.targetType, 'targetId': f.targetId},
        cursor: cursor,
      );

  /// `GET /admin/login-events` (audit.read), newest first.
  Future<PageResult<LoginEvent>> loginEvents(LoginEventFilters f, {String? cursor}) => _client.page(
        '/admin/login-events',
        LoginEvent.fromJson,
        query: {'adminId': f.adminId, 'email': f.email, 'outcome': f.outcome},
        cursor: cursor,
      );
}

/// One admin sign-in attempt (`GET /admin/login-events`). [admin] is null for
/// an unknown email. [reason] is for auditors only (the login screen never
/// says which check failed).
class LoginEvent {
  const LoginEvent({
    required this.id,
    this.admin,
    required this.outcome,
    required this.reason,
    this.ip,
    this.userAgent,
    this.createdAt,
  });

  final int id;
  final AdminRef? admin;
  final String outcome;
  final String reason;
  final String? ip;
  final String? userAgent;
  final DateTime? createdAt;

  bool get success => outcome == 'success';

  factory LoginEvent.fromJson(Json j) => LoginEvent(
        id: asInt(j['id']),
        admin: AdminRef.maybe(j['admin']),
        outcome: asString(j['outcome']),
        reason: asString(j['reason']),
        ip: asStringOrNull(j['ip']),
        userAgent: asStringOrNull(j['userAgent']),
        createdAt: asDate(j['createdAt']),
      );
}

class LoginEventFilters {
  const LoginEventFilters({this.adminId, this.email, this.outcome});

  final int? adminId;

  /// Matched on the server by keyed hash; never stored by this app.
  final String? email;

  /// `success` or `failure`; null for both.
  final String? outcome;
}


import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';

/// `AdminUserRow` from docs/api.md. Only masked PII (`mobileMasked`).
class AdminUserRow {
  const AdminUserRow({
    required this.id,
    required this.displayName,
    required this.mobileMasked,
    this.primaryRole,
    required this.roles,
    required this.status,
    required this.verificationStatus,
    required this.isMinor,
    required this.guardianConsentStatus,
    required this.onboardingStep,
    this.createdAt,
    this.lastLoginAt,
  });

  final int id;
  final String displayName;
  final String mobileMasked;
  final String? primaryRole;
  final List<String> roles;
  final String status;
  final String verificationStatus;
  final bool isMinor;
  final String guardianConsentStatus;
  final String onboardingStep;
  final DateTime? createdAt;
  final DateTime? lastLoginAt;

  factory AdminUserRow.fromJson(Json j) => AdminUserRow(
        id: asInt(j['id']),
        displayName: asString(j['displayName'], '(no name yet)'),
        mobileMasked: asString(j['mobileMasked']),
        primaryRole: asStringOrNull(j['primaryRole']),
        roles: asStringList(j['roles']),
        status: asString(j['status']),
        verificationStatus: asString(j['verificationStatus']),
        isMinor: asBool(j['isMinor']),
        guardianConsentStatus: asString(j['guardianConsentStatus']),
        onboardingStep: asString(j['onboardingStep']),
        createdAt: asDate(j['createdAt']),
        lastLoginAt: asDate(j['lastLoginAt']),
      );
}

/// `AdminUserDetail` without `verification.verifiedName`: the name is fetched
/// only when the admin presses "Reveal" (see [UsersApi.revealVerifiedName]).
class AdminUserDetail {
  const AdminUserDetail({
    required this.row,
    this.verification,
    this.guardian,
    required this.counts,
    this.suspendedReason,
  });

  final AdminUserRow row;

  /// status, provider, aadhaarLast4, gender, verifiedAt.
  final Json? verification;

  /// relation, mobileMasked, status, grantedAt.
  final Json? guardian;
  final Map<String, int> counts;
  final String? suspendedReason;

  factory AdminUserDetail.fromJson(Json j) {
    final v = asJsonOrNull(j['verification']);
    v?.remove('verifiedName'); // never kept in screen state until revealed
    final counts = <String, int>{};
    asJson(j['counts']).forEach((k, n) => counts[k] = asInt(n));
    return AdminUserDetail(
      row: AdminUserRow.fromJson(j),
      verification: v,
      guardian: asJsonOrNull(j['guardian']),
      counts: counts,
      suspendedReason: asStringOrNull(j['suspendedReason']),
    );
  }
}

class UserFilters {
  const UserFilters({this.q, this.role, this.status, this.verificationStatus, this.isMinor});

  final String? q;
  final String? role;
  final String? status;
  final String? verificationStatus;
  final bool? isMinor;

  Map<String, Object?> toQuery() => {
        'q': q,
        'role': role,
        'status': status,
        'verificationStatus': verificationStatus,
        'isMinor': isMinor,
      };
}

class UsersApi {
  UsersApi(this._client);

  final ApiClient _client;

  /// `GET /admin/users` (users.read).
  Future<PageResult<AdminUserRow>> search(UserFilters f, String? cursor) =>
      _client.page('/admin/users', AdminUserRow.fromJson, query: f.toQuery(), cursor: cursor);

  /// `GET /admin/users/:id` (users.read, audited).
  Future<AdminUserDetail> detail(int id) async => AdminUserDetail.fromJson(asJson(await _client.get('/admin/users/$id')));

  /// Reads the decrypted DigiLocker name on an explicit admin action. Uses
  /// `GET /admin/users/:id`, whose view the backend audits.
  Future<String?> revealVerifiedName(int id) async {
    final j = asJson(await _client.get('/admin/users/$id'));
    return asStringOrNull(asJson(j['verification'])['verifiedName']);
  }

  /// `POST /admin/users/:id/suspend` (users.write).
  Future<AdminUserRow> suspend(int id, String reason) async =>
      AdminUserRow.fromJson(asJson(await _client.post('/admin/users/$id/suspend', body: {'reason': reason})));

  /// `POST /admin/users/:id/guardian-consent/revoke` (users.write): withdraws a
  /// minor's granted guardian consent (e.g. the guardian asked support). The
  /// user is sent back to the guardian step. Returns the new consent status.
  Future<String> revokeGuardianConsent(int id, String reason) async {
    final j = asJson(await _client.post('/admin/users/$id/guardian-consent/revoke', body: {'reason': reason}));
    return asString(j['guardianConsentStatus'], 'revoked');
  }

  /// `POST /admin/users/:id/unsuspend` (users.write).
  Future<AdminUserRow> unsuspend(int id) async =>
      AdminUserRow.fromJson(asJson(await _client.post('/admin/users/$id/unsuspend')));
}

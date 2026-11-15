import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/auth/admin_me.dart';

const adminStatuses = ['active', 'disabled'];

class AdminRole {
  const AdminRole({required this.id, required this.name, required this.description, required this.permissions});

  final int id;
  final String name;
  final String description;
  final List<String> permissions;

  factory AdminRole.fromJson(Json j) => AdminRole(
        id: asInt(j['id']),
        name: asString(j['name']),
        description: asString(j['description']),
        permissions: asStringList(j['permissions']),
      );
}

/// One-time credentials from create / reset. Never stored, never logged; the
/// panel shows them once in a dialog and drops them when it closes.
class OneTimeCredentials {
  const OneTimeCredentials({required this.temporaryPassword, required this.totpUri});

  final String temporaryPassword;
  final String totpUri;

  factory OneTimeCredentials.fromJson(Json j) =>
      OneTimeCredentials(temporaryPassword: asString(j['temporaryPassword']), totpUri: asString(j['totpUri']));
}

class AdminsApi {
  AdminsApi(this._client);

  final ApiClient _client;

  /// `GET /admin/admins` (admins.manage) → plain list.
  Future<List<AdminMe>> admins() async => asList(await _client.get('/admin/admins'), AdminMe.fromJson);

  /// `POST /admin/admins` → `{admin, temporaryPassword, totpUri}`.
  Future<(AdminMe, OneTimeCredentials)> create({required String email, required String name, required int roleId}) async {
    final j = asJson(await _client.post('/admin/admins', body: {'email': email, 'name': name, 'roleId': roleId}));
    return (AdminMe.fromJson(asJson(j['admin'])), OneTimeCredentials.fromJson(j));
  }

  /// `PUT /admin/admins/:id` with only the changed fields.
  Future<AdminMe> update(int id, {String? name, int? roleId, String? status}) async => AdminMe.fromJson(asJson(await _client.put(
        '/admin/admins/$id',
        body: {
          if (name != null) 'name': name,
          if (roleId != null) 'roleId': roleId,
          if (status != null) 'status': status,
        },
      )));

  /// `POST /admin/admins/:id/reset-credentials`.
  Future<OneTimeCredentials> resetCredentials(int id) async =>
      OneTimeCredentials.fromJson(asJson(await _client.post('/admin/admins/$id/reset-credentials')));

  /// `GET /admin/roles` → plain list.
  Future<List<AdminRole>> roles() async => asList(await _client.get('/admin/roles'), AdminRole.fromJson);

  /// `POST /admin/roles`.
  Future<AdminRole> createRole({required String name, required String description, required List<String> permissions}) async =>
      AdminRole.fromJson(asJson(await _client.post('/admin/roles', body: {'name': name, 'description': description, 'permissions': permissions})));

  /// `PUT /admin/roles/:id` (the name cannot change).
  Future<AdminRole> updateRole(int id, {required String description, required List<String> permissions}) async =>
      AdminRole.fromJson(asJson(await _client.put('/admin/roles/$id', body: {'description': description, 'permissions': permissions})));
}

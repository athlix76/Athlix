import '../api/json.dart';

/// `AdminMe` / `AdminUser` from docs/api.md.
class AdminMe {
  const AdminMe({
    required this.id,
    required this.email,
    required this.name,
    required this.roleId,
    required this.roleName,
    required this.permissions,
    required this.status,
    this.lastLoginAt,
  });

  final int id;
  final String email;
  final String name;
  final int roleId;
  final String roleName;
  final List<String> permissions;
  final String status;
  final DateTime? lastLoginAt;

  factory AdminMe.fromJson(Json j) {
    final role = asJson(j['role']);
    return AdminMe(
      id: asInt(j['id']),
      email: asString(j['email']),
      name: asString(j['name']),
      roleId: asInt(role['id']),
      roleName: asString(role['name']),
      permissions: asStringList(j['permissions']),
      status: asString(j['status']),
      lastLoginAt: asDate(j['lastLoginAt']),
    );
  }
}

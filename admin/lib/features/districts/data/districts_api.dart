import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';

/// `AdminDistrict` = District + `{lgdCode, aliases, active}`.
class AdminDistrict {
  const AdminDistrict({required this.id, required this.stateId, required this.name, this.lgdCode, required this.aliases, required this.active});

  final int id;
  final int stateId;
  final String name;

  /// Local Government Directory code (Government of India), when known.
  final int? lgdCode;
  final List<String> aliases;
  final bool active;

  factory AdminDistrict.fromJson(Json j) => AdminDistrict(
        id: asInt(j['id']),
        stateId: asInt(j['stateId']),
        name: asString(j['name']),
        lgdCode: asIntOrNull(j['lgdCode']),
        aliases: asStringList(j['aliases']),
        active: asBool(j['active'], true),
      );
}

/// `AdminState` = State + `{lgdCode, active}`. States are seeded by migration.
class AdminState {
  const AdminState({required this.id, required this.code, required this.name, required this.isUT, this.lgdCode, required this.active});

  final int id;
  final String code;
  final String name;
  final bool isUT;
  final int? lgdCode;
  final bool active;

  factory AdminState.fromJson(Json j) => AdminState(
        id: asInt(j['id']),
        code: asString(j['code']),
        name: asString(j['name']),
        isUT: asBool(j['isUT']),
        lgdCode: asIntOrNull(j['lgdCode']),
        active: asBool(j['active'], true),
      );
}

class DistrictsApi {
  DistrictsApi(this._client);

  final ApiClient _client;

  /// `GET /admin/geo/states` (config.read). Every state, active or not, by name; a plain list.
  Future<List<AdminState>> states() async => asList(await _client.get('/admin/geo/states'), AdminState.fromJson);

  /// `GET /admin/geo/districts` (config.read).
  Future<PageResult<AdminDistrict>> list({int? stateId, String? q, bool includeInactive = false, String? cursor, int? limit}) => _client.page(
        '/admin/geo/districts',
        AdminDistrict.fromJson,
        query: {'stateId': stateId, 'q': q, 'includeInactive': includeInactive},
        cursor: cursor,
        limit: limit,
      );

  /// `POST /admin/geo/districts` (geo.write).
  Future<AdminDistrict> create({required int stateId, required String name, int? lgdCode, required List<String> aliases}) async =>
      AdminDistrict.fromJson(asJson(await _client.post('/admin/geo/districts', body: {
        'stateId': stateId,
        'name': name,
        'lgdCode': lgdCode,
        'aliases': aliases,
      })));

  /// `PUT /admin/geo/districts/:id` (geo.write).
  Future<AdminDistrict> update(int id, {required String name, int? lgdCode, required List<String> aliases, required bool active}) async =>
      AdminDistrict.fromJson(asJson(await _client.put('/admin/geo/districts/$id', body: {
        'name': name,
        'lgdCode': lgdCode,
        'aliases': aliases,
        'active': active,
      })));
}

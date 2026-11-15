import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';

const positionAppliesTo = ['player', 'coach', 'both'];
const statFieldTypes = ['int', 'decimal', 'text', 'duration_ms', 'percent'];

class Position {
  const Position({required this.id, required this.code, required this.name, required this.appliesTo, required this.sortOrder, required this.active});

  final int id;
  final String code;
  final String name;
  final String appliesTo;
  final int sortOrder;
  final bool active;

  factory Position.fromJson(Json j) => Position(
        id: asInt(j['id']),
        code: asString(j['code']),
        name: asString(j['name']),
        appliesTo: asString(j['appliesTo'], 'both'),
        sortOrder: asInt(j['sortOrder']),
        active: asBool(j['active'], true),
      );
}

class StatField {
  const StatField({required this.key, required this.label, required this.type, this.unit});

  final String key;
  final String label;
  final String type;
  final String? unit;

  factory StatField.fromJson(Json j) =>
      StatField(key: asString(j['key']), label: asString(j['label']), type: asString(j['type'], 'int'), unit: asStringOrNull(j['unit']));

  Json toJson() => {'key': key, 'label': label, 'type': type, 'unit': unit};
}

class StatTemplate {
  const StatTemplate({required this.id, this.positionId, required this.name, required this.version, required this.fields, required this.active});

  final int id;
  final int? positionId;
  final String name;
  final int version;
  final List<StatField> fields;
  final bool active;

  factory StatTemplate.fromJson(Json j) => StatTemplate(
        id: asInt(j['id']),
        positionId: asIntOrNull(j['positionId']),
        name: asString(j['name']),
        version: asInt(j['version'], 1),
        fields: asList(j['fields'], StatField.fromJson),
        active: asBool(j['active'], true),
      );
}

class AdminSport {
  const AdminSport({
    required this.id,
    required this.code,
    required this.name,
    required this.isTeamSport,
    required this.active,
    required this.sortOrder,
    required this.positions,
    required this.statTemplates,
  });

  final int id;
  final String code;
  final String name;
  final bool isTeamSport;
  final bool active;
  final int sortOrder;
  final List<Position> positions;
  final List<StatTemplate> statTemplates;

  factory AdminSport.fromJson(Json j) => AdminSport(
        id: asInt(j['id']),
        code: asString(j['code']),
        name: asString(j['name']),
        isTeamSport: asBool(j['isTeamSport']),
        active: asBool(j['active'], true),
        sortOrder: asInt(j['sortOrder']),
        positions: asList(j['positions'], Position.fromJson),
        statTemplates: asList(j['statTemplates'], StatTemplate.fromJson),
      );
}

class SportsApi {
  SportsApi(this._client);

  final ApiClient _client;

  /// `GET /admin/sports` (config.read), including inactive sports.
  Future<List<AdminSport>> list() async => asList(await _client.get('/admin/sports'), AdminSport.fromJson);

  /// `POST /admin/sports` (sports.write).
  Future<Object?> createSport({required String code, required String name, required bool isTeamSport, required int sortOrder}) =>
      _client.post('/admin/sports', body: {'code': code, 'name': name, 'isTeamSport': isTeamSport, 'sortOrder': sortOrder});

  /// `PUT /admin/sports/:id` (sports.write).
  Future<Object?> updateSport(int id, Json changes) => _client.put('/admin/sports/$id', body: changes);

  /// `POST /admin/sports/:id/positions` (sports.write).
  Future<Object?> createPosition(int sportId, {required String code, required String name, required String appliesTo, required int sortOrder}) =>
      _client.post('/admin/sports/$sportId/positions', body: {'code': code, 'name': name, 'appliesTo': appliesTo, 'sortOrder': sortOrder});

  /// `PUT /admin/positions/:id` (sports.write).
  Future<Object?> updatePosition(int id, Json changes) => _client.put('/admin/positions/$id', body: changes);

  /// `POST /admin/sports/:id/stat-templates` (sports.write).
  Future<Object?> createStatTemplate(int sportId, {int? positionId, required String name, required List<StatField> fields}) =>
      _client.post('/admin/sports/$sportId/stat-templates', body: {
        'positionId': positionId,
        'name': name,
        'fields': [for (final f in fields) f.toJson()],
      });

  /// `PUT /admin/stat-templates/:id` (sports.write). Changing `fields` bumps the version.
  Future<Object?> updateStatTemplate(int id, Json changes) => _client.put('/admin/stat-templates/$id', body: changes);
}

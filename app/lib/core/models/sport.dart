import '../json/json_utils.dart';

/// `SportRef`: `{"id", "code", "name"}`. Also used for positions in UserSummary.
class SportRef {
  const SportRef({required this.id, required this.code, required this.name});

  final int id;
  final String code;
  final String name;

  factory SportRef.fromJson(Map<String, dynamic> json) => SportRef(
        id: asInt(json['id']),
        code: asString(json['code']),
        name: asString(json['name']),
      );

  static SportRef? maybe(Object? json) {
    final m = asMapOrNull(json);
    return m == null ? null : SportRef.fromJson(m);
  }

  Map<String, dynamic> toJson() => {'id': id, 'code': code, 'name': name};
}

/// Position of a sport. `appliesTo`: `player | coach | both`.
class Position {
  const Position({
    required this.id,
    required this.code,
    required this.name,
    required this.appliesTo,
  });

  final int id;
  final String code;
  final String name;
  final String appliesTo;

  factory Position.fromJson(Map<String, dynamic> json) => Position(
        id: asInt(json['id']),
        code: asString(json['code']),
        name: asString(json['name']),
        appliesTo: asString(json['appliesTo'], 'both'),
      );

  bool appliesToRole(String? role) =>
      appliesTo == 'both' || role == null || appliesTo == role;
}

/// One field of a stat template. `type`: `int | decimal | text | duration_ms | percent`.
class StatField {
  const StatField({required this.key, required this.label, required this.type, this.unit});

  final String key;
  final String label;
  final String type;
  final String? unit;

  bool get isNumeric => type != 'text';

  factory StatField.fromJson(Map<String, dynamic> json) => StatField(
        key: asString(json['key']),
        label: asString(json['label']),
        type: asString(json['type'], 'text'),
        unit: asStringOrNull(json['unit']),
      );
}

class StatTemplate {
  const StatTemplate({
    required this.id,
    required this.positionId,
    required this.name,
    required this.version,
    required this.fields,
  });

  final int id;
  final int? positionId;
  final String name;
  final int version;
  final List<StatField> fields;

  factory StatTemplate.fromJson(Map<String, dynamic> json) => StatTemplate(
        id: asInt(json['id']),
        positionId: asIntOrNull(json['positionId']),
        name: asString(json['name']),
        version: asInt(json['version'], 1),
        fields: asList(json['fields'], StatField.fromJson),
      );
}

/// `Sport` from `GET /bootstrap` / `GET /sports`.
class Sport {
  const Sport({
    required this.id,
    required this.code,
    required this.name,
    required this.isTeamSport,
    required this.positions,
    required this.statTemplates,
  });

  final int id;
  final String code;
  final String name;
  final bool isTeamSport;
  final List<Position> positions;
  final List<StatTemplate> statTemplates;

  SportRef get ref => SportRef(id: id, code: code, name: name);

  factory Sport.fromJson(Map<String, dynamic> json) => Sport(
        id: asInt(json['id']),
        code: asString(json['code']),
        name: asString(json['name']),
        isTeamSport: asBool(json['isTeamSport']),
        positions: asList(json['positions'], Position.fromJson),
        statTemplates: asList(json['statTemplates'], StatTemplate.fromJson),
      );
}

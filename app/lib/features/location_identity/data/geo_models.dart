import '../../../core/json/json_utils.dart';

/// `State` from `GET /geo/states`.
class GeoState {
  const GeoState({required this.id, required this.code, required this.name, required this.isUT});

  final int id;
  final String code;
  final String name;
  final bool isUT;

  factory GeoState.fromJson(Map<String, dynamic> json) => GeoState(
        id: asInt(json['id']),
        code: asString(json['code']),
        name: asString(json['name']),
        isUT: asBool(json['isUT']),
      );

  static GeoState? maybe(Object? v) {
    final m = asMapOrNull(v);
    return m == null ? null : GeoState.fromJson(m);
  }

  Map<String, dynamic> toJson() => {'id': id, 'code': code, 'name': name, 'isUT': isUT};
}

/// `District` from `GET /geo/districts?stateId=`.
class District {
  const District({required this.id, required this.stateId, required this.name});

  final int id;
  final int stateId;
  final String name;

  factory District.fromJson(Map<String, dynamic> json) => District(
        id: asInt(json['id']),
        stateId: asInt(json['stateId']),
        name: asString(json['name']),
      );

  static District? maybe(Object? v) {
    final m = asMapOrNull(v);
    return m == null ? null : District.fromJson(m);
  }

  Map<String, dynamic> toJson() => {'id': id, 'stateId': stateId, 'name': name};
}

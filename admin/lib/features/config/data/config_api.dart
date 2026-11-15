import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/models/common.dart';

/// `ConfigEntry`. [value] is the decoded JSON value (int, double, bool,
/// String, List or Map), matching [valueType].
class ConfigEntry {
  const ConfigEntry({
    required this.key,
    required this.value,
    required this.valueType,
    required this.category,
    required this.description,
    required this.isPublic,
    required this.version,
    this.updatedAt,
  });

  final String key;
  final Object? value;
  final String valueType;
  final String category;
  final String description;
  final bool isPublic;
  final int version;
  final DateTime? updatedAt;

  factory ConfigEntry.fromJson(Json j) => ConfigEntry(
        key: asString(j['key']),
        value: j['value'],
        valueType: asString(j['valueType'], 'string'),
        category: asString(j['category'], 'other'),
        description: asString(j['description']),
        isPublic: asBool(j['isPublic']),
        version: asInt(j['version']),
        updatedAt: asDate(j['updatedAt']),
      );
}

class ConfigChange {
  const ConfigChange({this.oldValue, this.newValue, this.changedBy, this.changedAt});

  final Object? oldValue;
  final Object? newValue;
  final AdminRef? changedBy;
  final DateTime? changedAt;

  factory ConfigChange.fromJson(Json j) => ConfigChange(
        oldValue: j['oldValue'],
        newValue: j['newValue'],
        changedBy: AdminRef.maybe(j['changedBy']),
        changedAt: asDate(j['changedAt']),
      );
}

class ConfigApi {
  ConfigApi(this._client);

  final ApiClient _client;

  /// `GET /admin/config` (config.read).
  Future<List<ConfigEntry>> list({String? category}) async =>
      asList(await _client.get('/admin/config', query: {'category': category}), ConfigEntry.fromJson);

  /// `PUT /admin/config/:key` (config.write). 409 when [version] is stale,
  /// 422 when the value does not match the type.
  Future<ConfigEntry> update(String key, Object? value, int version) async => ConfigEntry.fromJson(
      asJson(await _client.put('/admin/config/${Uri.encodeComponent(key)}', body: {'value': value, 'version': version})));

  /// `GET /admin/config/:key/history` (config.read).
  Future<List<ConfigChange>> history(String key) async =>
      asList(await _client.get('/admin/config/${Uri.encodeComponent(key)}/history'), ConfigChange.fromJson);
}

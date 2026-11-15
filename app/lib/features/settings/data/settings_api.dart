import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/models/me.dart';
import '../../../core/providers.dart';
import 'settings_models.dart';

class SettingsApi {
  const SettingsApi(this._api);

  final ApiClient _api;

  Future<Settings> get() => _api.get('/me/settings', (d) => Settings.fromJson(asMap(d)));

  /// `PUT /me/settings` with a partial body. We always send a whole group
  /// (`notificationPrefs` or `privacy`), never a single nested key.
  Future<Settings> update(Map<String, dynamic> partial) =>
      _api.put('/me/settings', (d) => Settings.fromJson(asMap(d)), body: partial);

  /// `POST /me/roles` `{role}` → Me (adds a secondary role).
  Future<Me> addRole(String role) =>
      _api.post('/me/roles', (d) => Me.fromJson(asMap(d)), body: {'role': role});
}

final settingsApiProvider = Provider<SettingsApi>((ref) => SettingsApi(ref.watch(apiClientProvider)));

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/envelope.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import 'profile_models.dart';

class ProfileApi {
  const ProfileApi(this._api);

  final ApiClient _api;

  /// Records a profile view on the server unless it is your own profile or you are a minor.
  Future<Profile> get(int userId) => _api.get('/profiles/$userId', (d) => Profile.fromJson(asMap(d)));

  Future<Profile> update(ProfileUpdate update) =>
      _api.put('/me/profile', (d) => Profile.fromJson(asMap(d)), body: update.toJson());

  /// 403 `PLAN_REQUIRED` without benefit `see_profile_viewers`.
  Future<Paged<ProfileViewer>> viewers({String? cursor}) =>
      _api.page('/me/profile-viewers', ProfileViewer.fromJson, query: {'cursor': cursor});

  Future<StatsResult> stats(int userId) =>
      _api.get('/profiles/$userId/stats', (d) => StatsResult.fromJson(asMap(d)));

  Future<StatEntry> addStat(StatInput input) =>
      _api.post('/me/stats', (d) => StatEntry.fromJson(asMap(d)), body: input.toCreateJson());

  Future<StatEntry> updateStat(int id, StatInput input) =>
      _api.put('/me/stats/$id', (d) => StatEntry.fromJson(asMap(d)), body: input.toUpdateJson());

  Future<void> deleteStat(int id) => _api.delete('/me/stats/$id');

  Future<List<Achievement>> achievements(int userId) =>
      _api.get('/profiles/$userId/achievements', (d) => asList(d, Achievement.fromJson));

  Future<Achievement> addAchievement(AchievementInput input) =>
      _api.post('/me/achievements', (d) => Achievement.fromJson(asMap(d)), body: input.toJson());

  Future<Achievement> updateAchievement(int id, AchievementInput input) =>
      _api.put('/me/achievements/$id', (d) => Achievement.fromJson(asMap(d)), body: input.toJson());

  Future<void> deleteAchievement(int id) => _api.delete('/me/achievements/$id');
}

final profileApiProvider = Provider<ProfileApi>((ref) => ProfileApi(ref.watch(apiClientProvider)));

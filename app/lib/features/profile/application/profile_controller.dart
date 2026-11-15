import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/media/media_upload.dart';
import '../../../core/session/session_controller.dart';
import '../data/profile_api.dart';
import '../data/profile_models.dart';
import 'profile_providers.dart';

/// Writes to the signed-in user's profile, stats and achievements, then refreshes
/// the providers that show them.
class ProfileController {
  ProfileController(this._ref);

  final Ref _ref;

  ProfileApi get _api => _ref.read(profileApiProvider);

  int? get _myId => _ref.read(currentUserIdProvider);

  Future<Profile> updateProfile(ProfileUpdate update) async {
    final profile = await _api.update(update);
    _ref.invalidate(profileProvider(profile.user.id));
    // Name and photo also live on `Me`.
    await _ref.read(sessionProvider.notifier).refreshMe();
    return profile;
  }

  /// Uploads a file for [purpose] and returns its upload id (validated against config first).
  Future<int> upload(String purpose, UploadFile file) => _ref.read(mediaUploaderProvider).upload(purpose, file);

  String? validate(String purpose, UploadFile file) => _ref.read(mediaUploaderProvider).validate(purpose, file);

  Future<void> saveStat(StatInput input, {int? id}) async {
    if (id == null) {
      await _api.addStat(input);
    } else {
      await _api.updateStat(id, input);
    }
    _invalidateStats();
  }

  Future<void> deleteStat(int id) async {
    await _api.deleteStat(id);
    _invalidateStats();
  }

  Future<void> saveAchievement(AchievementInput input, {int? id}) async {
    if (id == null) {
      await _api.addAchievement(input);
    } else {
      await _api.updateAchievement(id, input);
    }
    _invalidateAchievements();
  }

  Future<void> deleteAchievement(int id) async {
    await _api.deleteAchievement(id);
    _invalidateAchievements();
  }

  void _invalidateStats() {
    final id = _myId;
    if (id != null) _ref.invalidate(statsProvider(id));
  }

  void _invalidateAchievements() {
    final id = _myId;
    if (id != null) _ref.invalidate(achievementsProvider(id));
  }
}

final profileControllerProvider = Provider<ProfileController>(ProfileController.new);

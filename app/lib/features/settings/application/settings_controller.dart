import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/models/me.dart';
import '../../../core/session/session_controller.dart';
import '../../notifications/application/push_service.dart';
import '../data/settings_api.dart';
import '../data/settings_models.dart';

/// The user's settings. Changes are optimistic: the switch moves at once and
/// rolls back if `PUT /me/settings` fails.
class SettingsController extends AutoDisposeAsyncNotifier<Settings> {
  @override
  Future<Settings> build() {
    ref.watch(currentUserIdProvider);
    return ref.watch(settingsApiProvider).get();
  }

  Future<void> setNotificationPrefs(NotificationPrefs prefs) =>
      _save((s) => s.copyWith(notificationPrefs: prefs), {'notificationPrefs': prefs.toJson()});

  Future<void> setPrivacy(PrivacySettings privacy) =>
      _save((s) => s.copyWith(privacy: privacy), {'privacy': privacy.toJson()});

  Future<void> _save(Settings Function(Settings) apply, Map<String, dynamic> body) async {
    final before = state.valueOrNull;
    if (before == null) return;
    state = AsyncData(apply(before));
    try {
      state = AsyncData(await ref.read(settingsApiProvider).update(body));
    } catch (_) {
      state = AsyncData(before);
      rethrow;
    }
  }
}

final settingsControllerProvider =
    AsyncNotifierProvider.autoDispose<SettingsController, Settings>(SettingsController.new);

/// Roles a user can add from settings (`POST /me/roles`).
abstract final class AddableRole {
  static const values = ['player', 'coach', 'organiser'];
}

/// Account actions: add a role, log out, delete the account.
class AccountActions {
  AccountActions(this._ref);

  final Ref _ref;

  Future<Me> addRole(String role) async {
    final me = await _ref.read(settingsApiProvider).addRole(role);
    _ref.read(sessionProvider.notifier).setMe(me);
    return me;
  }

  /// Unregisters this device's push token first (needs the session), then
  /// signs out.
  Future<void> logout() async {
    await _ref.read(pushServiceProvider).stop();
    await _ref.read(sessionProvider.notifier).logout();
  }

  /// `DELETE /me`. Push is stopped first while the session is still valid;
  /// if deletion fails, push is started again.
  Future<void> deleteAccount() async {
    final push = _ref.read(pushServiceProvider);
    await push.stop();
    try {
      await _ref.read(sessionProvider.notifier).deleteAccount();
    } catch (_) {
      await push.start();
      rethrow;
    }
  }
}

final accountActionsProvider = Provider<AccountActions>(AccountActions.new);

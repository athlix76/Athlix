import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../data/follow_api.dart';

/// Follow and block actions. Screens that show follow flags or counts listen to
/// [followChangesProvider] (or just reload) after a change.
class FollowController {
  FollowController(this._ref);

  final Ref _ref;

  FollowApi get _api => _ref.read(followApiProvider);

  Future<FollowState> setFollowing(int userId, bool follow) async {
    final state = follow ? await _api.follow(userId) : await _api.unfollow(userId);
    _bump();
    return state;
  }

  Future<void> block(int userId) async {
    await _api.block(userId);
    _bump();
  }

  Future<void> unblock(int userId) async {
    await _api.unblock(userId);
    _bump();
  }

  void _bump() => _ref.read(followChangesProvider.notifier).state++;
}

final followControllerProvider = Provider<FollowController>(FollowController.new);

/// Increments after every follow / block change, so other screens can refresh.
final followChangesProvider = StateProvider<int>((ref) => 0);

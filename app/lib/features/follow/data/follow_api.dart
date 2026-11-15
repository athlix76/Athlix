import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/envelope.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/models/user_summary.dart';
import '../../../core/providers.dart';

/// Response of `POST/DELETE /users/:id/follow`.
class FollowState {
  const FollowState({required this.following, required this.followersCount});

  final bool following;
  final int followersCount;

  factory FollowState.fromJson(Map<String, dynamic> json) => FollowState(
        following: asBool(json['following']),
        followersCount: asInt(json['followersCount']),
      );
}

class FollowApi {
  const FollowApi(this._api);

  final ApiClient _api;

  Future<FollowState> follow(int userId) =>
      _api.post('/users/$userId/follow', (d) => FollowState.fromJson(asMap(d)));

  Future<FollowState> unfollow(int userId) =>
      _api.deleteJson('/users/$userId/follow', (d) => FollowState.fromJson(asMap(d)));

  Future<Paged<UserSummary>> followers(int userId, {String? cursor}) =>
      _api.page('/users/$userId/followers', UserSummary.fromJson, query: {'cursor': cursor});

  Future<Paged<UserSummary>> following(int userId, {String? cursor}) =>
      _api.page('/users/$userId/following', UserSummary.fromJson, query: {'cursor': cursor});

  /// Also removes follows both ways (server side).
  Future<void> block(int userId) => _api.postVoid('/users/$userId/block');

  Future<void> unblock(int userId) => _api.delete('/users/$userId/block');

  Future<Paged<UserSummary>> blocks({String? cursor}) =>
      _api.page('/me/blocks', UserSummary.fromJson, query: {'cursor': cursor});
}

final followApiProvider = Provider<FollowApi>((ref) => FollowApi(ref.watch(apiClientProvider)));

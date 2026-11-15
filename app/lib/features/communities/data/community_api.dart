import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/envelope.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/models/user_summary.dart';
import '../../../core/providers.dart';
import 'community_models.dart';

/// REST calls of docs/api.md "Communities". Membership list and leaving use
/// the chat endpoints (a community is a conversation): see `chat/data/chat_api.dart`.
class CommunityApi {
  const CommunityApi(this._api);

  final ApiClient _api;

  Future<Community> create(CommunityInput input) =>
      _api.post('/communities', (d) => Community.fromJson(asMap(d)), body: input.toJson());

  /// Public, active communities.
  Future<Paged<Community>> list({String? q, int? sportId, String? state, String? cursor}) => _api.page(
        '/communities',
        Community.fromJson,
        query: {'q': q, 'sportId': sportId, 'state': state, 'cursor': cursor},
      );

  Future<Paged<Community>> mine({String? cursor}) =>
      _api.page('/me/communities', Community.fromJson, query: {'cursor': cursor});

  Future<Community> get(int id) => _api.get('/communities/$id', (d) => Community.fromJson(asMap(d)));

  /// Partial update: [body] holds only the changed `CommunityInput` fields.
  Future<Community> update(int id, Map<String, dynamic> body) =>
      _api.put('/communities/$id', (d) => Community.fromJson(asMap(d)), body: body);

  /// Owner only; the community is archived.
  Future<void> delete(int id) => _api.delete('/communities/$id');

  /// Returns the new `myStatus` (`active` or `pending`).
  Future<String> join(int id) => _api.post(
        '/communities/$id/join',
        (d) => asString(asMap(d)['myStatus'], MembershipStatus.pending),
        body: const <String, dynamic>{},
      );

  Future<Paged<JoinRequest>> joinRequests(int id, {String? cursor}) =>
      _api.page('/communities/$id/join-requests', JoinRequest.fromJson, query: {'cursor': cursor});

  Future<void> approve(int id, int userId) => _api.postVoid('/communities/$id/join-requests/$userId/approve');

  Future<void> reject(int id, int userId) => _api.postVoid('/communities/$id/join-requests/$userId/reject');

  Future<InviteResult> invite(int id, List<int> userIds) => _api.post(
        '/communities/$id/invites',
        (d) => InviteResult.fromJson(asMap(d)),
        body: {'userIds': userIds},
      );

  Future<Community> acceptInvite(int id) => _api.post(
        '/communities/$id/invites/accept',
        (d) => Community.fromJson(asMap(d)),
        body: const <String, dynamic>{},
      );

  Future<void> removeMember(int id, int userId) => _api.postVoid('/communities/$id/members/$userId/remove');

  Future<void> banMember(int id, int userId) => _api.postVoid('/communities/$id/members/$userId/ban');

  /// Owner only. [userId] must be an active admin; the caller becomes an admin.
  Future<void> transferOwnership(int id, int userId) =>
      _api.postVoid('/communities/$id/transfer-ownership', body: {'userId': userId});

  /// Owner only. [role] is `admin` or `member`.
  Future<void> setRole(int id, int userId, String role) =>
      _api.putVoid('/communities/$id/members/$userId/role', body: {'role': role});

  /// People search for the invite picker (`PersonResult` read as its `UserSummary` part).
  Future<Paged<UserSummary>> searchPeople(String q, {String? cursor}) =>
      _api.page('/search/people', UserSummary.fromJson, query: {'q': q, 'cursor': cursor});
}

final communityApiProvider = Provider<CommunityApi>((ref) => CommunityApi(ref.watch(apiClientProvider)));

import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';
import '../../../core/models/common.dart';

/// Community status filter values. The contract does not list them; these
/// follow the moderation status of `conversations` (see README contract gaps).
const communityStatuses = ['active', 'hidden', 'removed'];

/// `Community + {status}`.
class AdminCommunity {
  const AdminCommunity({
    required this.id,
    required this.title,
    required this.description,
    this.sport,
    this.city,
    this.state,
    required this.visibility,
    required this.joinPolicy,
    required this.memberCount,
    this.owner,
    this.createdAt,
    required this.status,
  });

  final int id;
  final String title;
  final String description;
  final SportRef? sport;
  final String? city;
  final String? state;
  final String visibility;
  final String joinPolicy;
  final int memberCount;
  final UserSummary? owner;
  final DateTime? createdAt;
  final String status;

  factory AdminCommunity.fromJson(Json j) => AdminCommunity(
        id: asInt(j['id']),
        title: asString(j['title']),
        description: asString(j['description']),
        sport: SportRef.maybe(j['sport']),
        city: asStringOrNull(j['city']),
        state: asStringOrNull(j['state']),
        visibility: asString(j['visibility']),
        joinPolicy: asString(j['joinPolicy']),
        memberCount: asInt(j['memberCount']),
        owner: UserSummary.maybe(j['owner']),
        createdAt: asDate(j['createdAt']),
        status: asString(j['status'], 'active'),
      );
}

class CommunitiesApi {
  CommunitiesApi(this._client);

  final ApiClient _client;

  /// `GET /admin/communities` (communities.moderate).
  Future<PageResult<AdminCommunity>> list({String? q, String? status, String? cursor}) =>
      _client.page('/admin/communities', AdminCommunity.fromJson, query: {'q': q, 'status': status}, cursor: cursor);

  /// `POST /admin/communities/:id/remove` (communities.moderate) → 204.
  Future<void> remove(int id, String reason) async {
    await _client.post('/admin/communities/$id/remove', body: {'reason': reason});
  }
}

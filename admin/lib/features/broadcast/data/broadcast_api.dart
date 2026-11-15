import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';

const broadcastRoles = ['player', 'coach', 'organiser'];

class BroadcastAudience {
  const BroadcastAudience({this.roles = const [], this.sportId, this.state, this.includeMinors = false});

  /// Empty = every role.
  final List<String> roles;
  final int? sportId;
  final String? state;
  final bool includeMinors;

  Json toJson() => {
        if (roles.isNotEmpty) 'roles': roles,
        if (sportId != null) 'sportId': sportId,
        if (state != null && state!.isNotEmpty) 'state': state,
        'includeMinors': includeMinors,
      };
}

class BroadcastApi {
  BroadcastApi(this._client);

  final ApiClient _client;

  /// `POST /admin/notifications/broadcast` (notifications.broadcast) → number of recipients queued.
  Future<int> send({required String title, required String body, required BroadcastAudience audience}) async =>
      asInt(asJson(await _client.post('/admin/notifications/broadcast', body: {
        'title': title,
        'body': body,
        'audience': audience.toJson(),
      }))['queued']);
}

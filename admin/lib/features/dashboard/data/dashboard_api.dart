import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';

/// `Dashboard` from docs/api.md: groups of named counters.
class Dashboard {
  const Dashboard(this.groups);

  /// group → counter → value, e.g. `users.total`, `revenue.listingPaidPaiseLast30Days`.
  final Map<String, Map<String, int>> groups;

  int value(String group, String key) => groups[group]?[key] ?? 0;

  factory Dashboard.fromJson(Json j) {
    final out = <String, Map<String, int>>{};
    j.forEach((group, v) {
      final m = <String, int>{};
      asJson(v).forEach((k, n) => m[k] = asInt(n));
      out[group] = m;
    });
    return Dashboard(out);
  }
}

class DashboardApi {
  DashboardApi(this._client);

  final ApiClient _client;

  /// `GET /admin/dashboard` (dashboard.read).
  Future<Dashboard> load() async => Dashboard.fromJson(asJson(await _client.get('/admin/dashboard')));
}

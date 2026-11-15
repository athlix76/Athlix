import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/providers.dart';
import '../../../core/json/json_utils.dart';
import 'ad_models.dart';

class AdApi {
  const AdApi(this._api);

  final ApiClient _api;

  /// `[]` when ads are disabled or hidden for minors (server decides).
  Future<List<Ad>> list(AdQuery q) => _api.get(
        '/ads',
        (d) => asList(d, Ad.fromJson),
        query: {'placement': q.placement, 'sportId': q.sportId, 'state': q.state},
      );

  Future<void> impression(int adId) => _api.postVoid('/ads/$adId/impression');

  Future<void> click(int adId) => _api.postVoid('/ads/$adId/click');
}

final adApiProvider = Provider<AdApi>((ref) => AdApi(ref.watch(apiClientProvider)));

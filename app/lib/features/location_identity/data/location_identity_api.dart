import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import 'geo_models.dart';
import 'location_identity_models.dart';

class LocationIdentityApi {
  const LocationIdentityApi(this._api);

  final ApiClient _api;

  Future<List<GeoState>> states() =>
      _api.get('/geo/states', (d) => asList(d, GeoState.fromJson));

  Future<List<District>> districts(int stateId) => _api.get(
        '/geo/districts',
        (d) => asList(d, District.fromJson),
        query: {'stateId': stateId},
      );

  Future<LocationIdentities> mine() =>
      _api.get('/me/location-identities', (d) => LocationIdentities.fromJson(asMap(d)));

  Future<LocationIdentity> submit(LocationIdentityInput input) => _api.post(
        '/me/location-identities',
        (d) => LocationIdentity.fromJson(asMap(d)),
        body: input.toJson(),
      );

  Future<void> withdraw(int id) => _api.delete('/me/location-identities/$id');
}

final locationIdentityApiProvider =
    Provider<LocationIdentityApi>((ref) => LocationIdentityApi(ref.watch(apiClientProvider)));

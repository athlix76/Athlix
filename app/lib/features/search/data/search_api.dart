import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/envelope.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import 'search_models.dart';

class SearchApi {
  const SearchApi(this._api);

  final ApiClient _api;

  Future<SearchResults> all(String q) =>
      _api.get('/search', (d) => SearchResults.fromJson(asMap(d)), query: {'q': q});

  Future<Paged<PersonResult>> people(PeopleFilters filters, {String? cursor, int? limit}) => _api.page(
        '/search/people',
        PersonResult.fromJson,
        query: {...filters.toQuery(), 'cursor': cursor, 'limit': limit},
      );
}

final searchApiProvider = Provider<SearchApi>((ref) => SearchApi(ref.watch(apiClientProvider)));

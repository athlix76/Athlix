import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../data/search_api.dart';
import '../data/search_models.dart';

/// Minimum characters before `GET /search` is called.
const kMinSearchLength = 2;

/// Combined search for one query string (trimmed by the caller).
final searchResultsProvider = FutureProvider.autoDispose.family<SearchResults, String>((ref, q) {
  if (q.length < kMinSearchLength) return Future.value(SearchResults.empty);
  return ref.watch(searchApiProvider).all(q);
});

/// Filters of the people search screen (kept while the app runs).
final peopleFiltersProvider = StateProvider<PeopleFilters>((ref) => const PeopleFilters());

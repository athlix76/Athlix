import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/districts_api.dart';

final districtsApiProvider = Provider<DistrictsApi>((ref) => DistrictsApi(ref.watch(apiClientProvider)));

/// Active districts of one state, for pickers (first page of up to 100).
final districtsOfStateProvider = FutureProvider.autoDispose.family<List<AdminDistrict>, int>((ref, stateId) async {
  final page = await ref.watch(districtsApiProvider).list(stateId: stateId, limit: 100);
  return page.items;
});

/// Every state (active and inactive), by name, for the state pickers.
final adminStatesProvider = FutureProvider.autoDispose<List<AdminState>>((ref) => ref.watch(districtsApiProvider).states());

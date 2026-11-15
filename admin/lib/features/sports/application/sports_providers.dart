import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/sports_api.dart';

final sportsApiProvider = Provider<SportsApi>((ref) => SportsApi(ref.watch(apiClientProvider)));

/// All sports (active and inactive). Also used by the fee rule, ads and
/// broadcast screens for their sport dropdowns.
final adminSportsProvider = FutureProvider.autoDispose<List<AdminSport>>((ref) => ref.watch(sportsApiProvider).list());

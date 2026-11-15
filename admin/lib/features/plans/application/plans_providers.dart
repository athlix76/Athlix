import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/plans_api.dart';

final plansApiProvider = Provider<PlansApi>((ref) => PlansApi(ref.watch(apiClientProvider)));

/// Also used by the subscriptions screen for its plan filter and grant form.
final plansProvider = FutureProvider.autoDispose<List<Plan>>((ref) => ref.watch(plansApiProvider).list());

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/dashboard_api.dart';

final dashboardApiProvider = Provider<DashboardApi>((ref) => DashboardApi(ref.watch(apiClientProvider)));

final dashboardProvider = FutureProvider.autoDispose<Dashboard>((ref) => ref.watch(dashboardApiProvider).load());

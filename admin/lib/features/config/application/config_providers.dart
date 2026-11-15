import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/config_api.dart';

final configApiProvider = Provider<ConfigApi>((ref) => ConfigApi(ref.watch(apiClientProvider)));

final configEntriesProvider = FutureProvider.autoDispose<List<ConfigEntry>>((ref) => ref.watch(configApiProvider).list());

final configHistoryProvider = FutureProvider.autoDispose.family<List<ConfigChange>, String>(
  (ref, key) => ref.watch(configApiProvider).history(key),
);

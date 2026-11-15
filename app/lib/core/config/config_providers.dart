import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:package_info_plus/package_info_plus.dart';

import '../api/api_signals.dart';
import '../json/json_utils.dart';
import '../models/sport.dart';
import '../providers.dart';
import 'app_config.dart';
import 'bootstrap.dart';
import 'version.dart';

/// `GET /bootstrap`, loaded once at start and re-loaded from the
/// maintenance / error screens. A `MAINTENANCE` error from any call flips
/// [Bootstrap.maintenance] on.
class BootstrapController extends AsyncNotifier<Bootstrap> {
  @override
  Future<Bootstrap> build() async {
    final signals = ref.watch(apiSignalsProvider);
    final sub = signals.stream.listen((s) {
      if (s.type == ApiSignalType.maintenance) setMaintenance(true);
    });
    ref.onDispose(sub.cancel);
    return _fetch();
  }

  Future<Bootstrap> _fetch() => ref
      .read(apiClientProvider)
      .get('/bootstrap', (d) => Bootstrap.fromJson(asMap(d)), auth: false);

  Future<void> reload() async {
    state = const AsyncLoading<Bootstrap>();
    state = await AsyncValue.guard(_fetch);
  }

  void setMaintenance(bool on) {
    final current = state.valueOrNull;
    if (current != null && current.maintenance != on) {
      state = AsyncData(current.copyWith(maintenance: on));
    }
  }
}

final bootstrapProvider =
    AsyncNotifierProvider<BootstrapController, Bootstrap>(BootstrapController.new);

/// Public config; [AppConfig.empty] until bootstrap has loaded.
final appConfigProvider = Provider<AppConfig>(
  (ref) => ref.watch(bootstrapProvider).valueOrNull?.config ?? AppConfig.empty,
);

final sportsProvider = Provider<List<Sport>>(
  (ref) => ref.watch(bootstrapProvider).valueOrNull?.sports ?? const <Sport>[],
);

/// Installed app version (`1.2.3`), or null if the platform could not tell.
final appVersionProvider = FutureProvider<String?>((ref) async {
  try {
    final info = await PackageInfo.fromPlatform();
    return info.version;
  } catch (_) {
    return null;
  }
});

/// What the start-up gate needs to decide where the user may go.
class AppGate {
  const AppGate({
    required this.ready,
    required this.forceUpdate,
    required this.maintenance,
    this.error,
  });

  final bool ready;
  final bool forceUpdate;
  final bool maintenance;
  final Object? error;
}

final appGateProvider = Provider<AppGate>((ref) {
  final boot = ref.watch(bootstrapProvider);
  final version = ref.watch(appVersionProvider);
  final b = boot.valueOrNull;
  if (b == null || version.isLoading) {
    return AppGate(ready: false, forceUpdate: false, maintenance: false, error: boot.error);
  }
  final current = version.valueOrNull;
  final minimum = defaultTargetPlatform == TargetPlatform.iOS ? b.minVersionIos : b.minVersionAndroid;
  return AppGate(
    ready: true,
    forceUpdate: current != null && needsForceUpdate(current, minimum),
    maintenance: b.maintenance,
  );
});

import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api/api_client.dart';
import 'api/api_signals.dart';
import 'auth/token_store.dart';
import 'env.dart';

/// Root singletons. Override [tokenStoreProvider] / [apiClientProvider] in tests.
final tokenStoreProvider = Provider<TokenStore>((ref) => SecureTokenStore());

final apiSignalsProvider = Provider<ApiSignals>((ref) {
  final signals = ApiSignals();
  ref.onDispose(signals.close);
  return signals;
});

final apiClientProvider = Provider<ApiClient>((ref) {
  final signals = ref.watch(apiSignalsProvider);
  return ApiClient.create(
    baseUrl: Env.apiBaseUrl,
    tokenStore: ref.watch(tokenStoreProvider),
    listener: ApiListener(
      onSessionExpired: () => signals.emit(const ApiSignal(ApiSignalType.sessionExpired)),
      onOnboardingRequired: (step) =>
          signals.emit(ApiSignal(ApiSignalType.onboardingRequired, nextStep: step)),
      onSuspended: () => signals.emit(const ApiSignal(ApiSignalType.suspended)),
      onMaintenance: () => signals.emit(const ApiSignal(ApiSignalType.maintenance)),
    ),
  );
});

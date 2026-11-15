import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api/api_client.dart';
import 'auth/token_store.dart';
import 'config/env.dart';

/// In-memory token holder (one per tab).
final tokenStoreProvider = Provider<TokenStore>((ref) => TokenStore());

/// The shared API client.
final apiClientProvider = Provider<ApiClient>(
  (ref) => ApiClient(baseUrl: Env.apiBaseUrl, tokens: ref.watch(tokenStoreProvider)),
);

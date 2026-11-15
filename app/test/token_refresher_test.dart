import 'dart:async';

import 'package:athlix/core/api/api_exception.dart';
import 'package:athlix/core/auth/token_pair.dart';
import 'package:athlix/core/auth/token_refresher.dart';
import 'package:athlix/core/auth/token_store.dart';
import 'package:flutter_test/flutter_test.dart';

class MemoryTokenStore implements TokenStore {
  MemoryTokenStore([this.tokens]);
  TokenPair? tokens;
  int saves = 0;
  int clears = 0;

  @override
  Future<TokenPair?> read() async => tokens;

  @override
  Future<void> save(TokenPair t) async {
    saves++;
    tokens = t;
  }

  @override
  Future<void> clear() async {
    clears++;
    tokens = null;
  }
}

TokenPair pair(String access, String refresh) => TokenPair(
      accessToken: access,
      accessTokenExpiresAt: null,
      refreshToken: refresh,
      refreshTokenExpiresAt: null,
    );

void main() {
  test('concurrent refreshes share one call (single flight)', () async {
    final store = MemoryTokenStore(pair('a1', 'r1'));
    var calls = 0;
    final gate = Completer<TokenPair>();
    final refresher = TokenRefresher(
      store: store,
      call: (rt) {
        calls++;
        expect(rt, 'r1');
        return gate.future;
      },
    );

    final futures = List.generate(5, (_) => refresher.refresh());
    expect(refresher.isRefreshing, isTrue);
    gate.complete(pair('a2', 'r2'));
    final results = await Future.wait(futures);

    expect(calls, 1);
    expect(results.every((r) => r is RefreshSuccess), isTrue);
    expect(identical(results.first, results.last), isTrue);
    expect(store.tokens!.refreshToken, 'r2');
    expect(store.saves, 1);
  });

  test('a later refresh after completion starts a new call', () async {
    final store = MemoryTokenStore(pair('a1', 'r1'));
    var calls = 0;
    final refresher = TokenRefresher(
      store: store,
      call: (rt) async {
        calls++;
        return pair('a${calls + 1}', 'r${calls + 1}');
      },
    );
    await refresher.refresh();
    await Future<void>.delayed(Duration.zero);
    expect(refresher.isRefreshing, isFalse);
    await refresher.refresh();
    expect(calls, 2);
    expect(store.tokens!.refreshToken, 'r3');
  });

  test('no stored refresh token is rejected without a call', () async {
    var calls = 0;
    final refresher = TokenRefresher(
      store: MemoryTokenStore(),
      call: (_) async {
        calls++;
        return pair('a', 'r');
      },
    );
    expect(await refresher.refresh(), isA<RefreshRejected>());
    expect(calls, 0);
  });

  test('401 from refresh clears tokens and is rejected', () async {
    final store = MemoryTokenStore(pair('a1', 'r1'));
    final refresher = TokenRefresher(
      store: store,
      call: (_) async => throw const ApiException(code: ApiErrorCode.unauthenticated, message: 'x'),
    );
    expect(await refresher.refresh(), isA<RefreshRejected>());
    expect(store.tokens, isNull);
    expect(store.clears, 1);
  });

  test('network failure keeps the session', () async {
    final store = MemoryTokenStore(pair('a1', 'r1'));
    final refresher = TokenRefresher(
      store: store,
      call: (_) async => throw ApiException.network(),
    );
    expect(await refresher.refresh(), isA<RefreshFailed>());
    expect(store.tokens!.refreshToken, 'r1');
    expect(store.clears, 0);
  });

  test('an invalid (empty) pair from the server is rejected', () async {
    final store = MemoryTokenStore(pair('a1', 'r1'));
    final refresher = TokenRefresher(store: store, call: (_) async => pair('', ''));
    expect(await refresher.refresh(), isA<RefreshRejected>());
    expect(store.tokens, isNull);
  });

  test('TokenPair.toString is redacted', () {
    expect(pair('secret-access', 'secret-refresh').toString(), isNot(contains('secret')));
  });
}

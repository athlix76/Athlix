import '../api/api_exception.dart';
import 'token_pair.dart';
import 'token_store.dart';

/// Calls `POST /auth/refresh` with the stored refresh token and returns the new pair.
typedef RefreshCall = Future<TokenPair> Function(String refreshToken);

sealed class RefreshResult {
  const RefreshResult();
}

/// New tokens were issued and saved.
class RefreshSuccess extends RefreshResult {
  const RefreshSuccess(this.tokens);
  final TokenPair tokens;
}

/// The server refused the refresh token (expired, revoked, reused): sign out.
class RefreshRejected extends RefreshResult {
  const RefreshRejected();
}

/// The refresh could not be attempted (offline, 5xx). Keep the session.
class RefreshFailed extends RefreshResult {
  const RefreshFailed(this.error);
  final Object error;
}

/// Single-flight refresh-token rotation.
///
/// Refresh tokens rotate and reusing an old one revokes the whole session
/// family (docs/api.md "Auth"). So when several requests get a 401 at the same
/// time, exactly one refresh call may run; every caller awaits that same call.
class TokenRefresher {
  TokenRefresher({required TokenStore store, required RefreshCall call})
      : _store = store,
        _call = call;

  final TokenStore _store;
  final RefreshCall _call;
  Future<RefreshResult>? _inFlight;

  bool get isRefreshing => _inFlight != null;

  Future<RefreshResult> refresh() {
    final existing = _inFlight;
    if (existing != null) return existing;
    final future = _run();
    _inFlight = future;
    // _run never throws, so this cannot produce an unhandled error.
    future.whenComplete(() {
      if (identical(_inFlight, future)) _inFlight = null;
    });
    return future;
  }

  Future<RefreshResult> _run() async {
    TokenPair? current;
    try {
      current = await _store.read();
    } catch (e) {
      return RefreshFailed(e);
    }
    final refreshToken = current?.refreshToken;
    if (refreshToken == null || refreshToken.isEmpty) {
      return const RefreshRejected();
    }
    try {
      final next = await _call(refreshToken);
      if (!next.isValid) {
        await _store.clear();
        return const RefreshRejected();
      }
      await _store.save(next);
      return RefreshSuccess(next);
    } on ApiException catch (e) {
      final refused = e.code == ApiErrorCode.unauthenticated ||
          e.code == ApiErrorCode.forbidden ||
          e.code == ApiErrorCode.accountSuspended ||
          e.code == ApiErrorCode.validationFailed ||
          e.code == ApiErrorCode.badRequest;
      if (refused) {
        await _store.clear();
        return const RefreshRejected();
      }
      return RefreshFailed(e);
    } catch (e) {
      return RefreshFailed(e);
    }
  }
}

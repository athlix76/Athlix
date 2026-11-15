import 'package:dio/dio.dart';

import 'token_refresher.dart';
import 'token_store.dart';

/// Adds `Authorization: Bearer <accessToken>` and handles 401:
///
/// 1. If the request was sent with an access token that has since been
///    replaced (another request already refreshed), retry with the new one.
/// 2. Otherwise refresh once through [TokenRefresher] (single-flight) and retry.
/// 3. Refresh refused, or the retry gets 401 again → [onSessionExpired].
///
/// Requests marked with [skipAuth] (OTP, refresh) are left alone.
class AuthInterceptor extends Interceptor {
  AuthInterceptor({
    required TokenStore store,
    required TokenRefresher refresher,
    required Dio retryDio,
    required void Function() onSessionExpired,
  })  : _store = store,
        _refresher = refresher,
        _retryDio = retryDio,
        _onSessionExpired = onSessionExpired;

  /// Put `true` under this key in `Options.extra` to send no token.
  static const String skipAuth = 'athlix.skipAuth';
  static const String _retried = 'athlix.retried';
  static const String _header = 'Authorization';

  final TokenStore _store;
  final TokenRefresher _refresher;
  final Dio _retryDio;
  final void Function() _onSessionExpired;

  @override
  void onRequest(
      RequestOptions options, RequestInterceptorHandler handler) async {
    if (options.extra[skipAuth] != true) {
      final tokens = await _store.read();
      if (tokens != null && tokens.accessToken.isNotEmpty) {
        options.headers[_header] = 'Bearer ${tokens.accessToken}';
      } else {
        options.headers.remove(_header);
      }
    }
    handler.next(options);
  }

  @override
  void onError(DioException err, ErrorInterceptorHandler handler) async {
    final opts = err.requestOptions;
    if (err.response?.statusCode != 401 || opts.extra[skipAuth] == true) {
      handler.next(err);
      return;
    }
    if (opts.extra[_retried] == true) {
      // Refreshed once already and still 401: the session is gone.
      _onSessionExpired();
      handler.next(err);
      return;
    }

    final current = await _store.read();
    if (current == null || current.refreshToken.isEmpty) {
      _onSessionExpired();
      handler.next(err);
      return;
    }

    final String accessToken;
    final sentWith = opts.headers[_header];
    if (sentWith != null && sentWith != 'Bearer ${current.accessToken}') {
      // Someone else refreshed while this request was in flight.
      accessToken = current.accessToken;
    } else {
      final result = await _refresher.refresh();
      switch (result) {
        case RefreshSuccess(:final tokens):
          accessToken = tokens.accessToken;
        case RefreshRejected():
          _onSessionExpired();
          handler.next(err);
          return;
        case RefreshFailed():
          handler.next(err);
          return;
      }
    }

    if (accessToken.isEmpty) {
      handler.next(err);
      return;
    }

    final retry = opts.copyWith(
      headers: <String, dynamic>{
        ...opts.headers,
        _header: 'Bearer $accessToken'
      },
      extra: <String, dynamic>{...opts.extra, _retried: true},
    );
    try {
      final response = await _retryDio.fetch<dynamic>(retry);
      handler.resolve(response);
    } on DioException catch (e) {
      handler.next(e);
    }
  }
}

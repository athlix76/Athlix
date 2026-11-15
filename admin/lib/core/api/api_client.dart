import 'package:dio/dio.dart';

import '../auth/token_store.dart';
import '../config/env.dart';
import 'api_error.dart';
import 'json.dart';
import 'page.dart';

/// The only HTTP client of the panel. Attaches the admin access token, refreshes
/// it before expiry and once on a 401, unwraps `{"data": ...}`, and turns every
/// failure into an [ApiError]. It never logs requests, bodies or tokens.
class ApiClient {
  ApiClient({required this.baseUrl, required this.tokens, Dio? dio, Dio? bare})
      : _dio = dio ?? Dio(_options(baseUrl)),
        _bare = bare ?? Dio(_options(baseUrl)) {
    _dio.interceptors.add(_AuthInterceptor(this));
  }

  static BaseOptions _options(String baseUrl) => BaseOptions(
        baseUrl: baseUrl,
        connectTimeout: const Duration(seconds: 20),
        receiveTimeout: const Duration(seconds: 30),
        contentType: Headers.jsonContentType,
        responseType: ResponseType.json,
      );

  final String baseUrl;
  final TokenStore tokens;
  final Dio _dio;

  /// No interceptors: used for refresh and the one retry after a refresh.
  final Dio _bare;

  static const _loginPath = '/admin/auth/login';
  static const _refreshPath = '/admin/auth/refresh';

  static bool _skipsAuth(String path) => path == _loginPath || path == _refreshPath;

  Future<bool>? _inflightRefresh;

  /// Refreshes the token pair once, even if many requests ask at the same time.
  Future<bool> refresh() {
    return _inflightRefresh ??= _doRefresh().whenComplete(() {
      _inflightRefresh = null;
    });
  }

  Future<bool> _doRefresh() async {
    final rt = tokens.refreshToken;
    if (rt == null || tokens.refreshExpired()) {
      tokens.expire();
      return false;
    }
    try {
      final res = await _bare.post<dynamic>(_refreshPath, data: {'refreshToken': rt});
      tokens.set(TokenPair.fromJson(asJson(asJson(res.data)['data'])));
      tokens.onRefreshed?.call();
      return true;
    } on DioException catch (e) {
      final s = e.response?.statusCode;
      if (s != null && s >= 400 && s < 500) {
        tokens.expire(); // revoked, reused or expired: the session is over
      }
      return false;
    } catch (_) {
      return false;
    }
  }

  Future<Response<dynamic>> _send(Future<Response<dynamic>> Function() call) async {
    try {
      return await call();
    } on DioException catch (e) {
      throw ApiError.fromDio(e);
    }
  }

  /// GET → the `data` of the envelope.
  Future<dynamic> get(String path, {Map<String, Object?> query = const {}}) async {
    final res = await _send(() => _dio.get<dynamic>(path, queryParameters: cleanQuery(query)));
    return asJson(res.data)['data'];
  }

  /// GET a cursor page.
  Future<PageResult<T>> page<T>(
    String path,
    T Function(Json) parse, {
    Map<String, Object?> query = const {},
    String? cursor,
    int? limit,
  }) async {
    final q = <String, Object?>{...query, 'cursor': cursor, 'limit': limit};
    final res = await _send(() => _dio.get<dynamic>(path, queryParameters: cleanQuery(q)));
    return PageResult.fromEnvelope(res.data, parse);
  }

  Future<dynamic> post(String path, {Object? body, Map<String, Object?> query = const {}}) async {
    final res = await _send(() => _dio.post<dynamic>(path,
        data: body ?? const <String, dynamic>{}, queryParameters: cleanQuery(query)));
    return _dataOrNull(res);
  }

  Future<dynamic> put(String path, {Object? body}) async {
    final res = await _send(() => _dio.put<dynamic>(path, data: body ?? const <String, dynamic>{}));
    return _dataOrNull(res);
  }

  Future<dynamic> patch(String path, {Object? body}) async {
    final res = await _send(() => _dio.patch<dynamic>(path, data: body ?? const <String, dynamic>{}));
    return _dataOrNull(res);
  }

  Future<dynamic> delete(String path, {Object? body}) async {
    final res = await _send(() => _dio.delete<dynamic>(path, data: body));
    return _dataOrNull(res);
  }

  static dynamic _dataOrNull(Response<dynamic> res) {
    if (res.statusCode == 204) return null;
    final d = res.data;
    return d is Map ? d['data'] : null;
  }
}

class _AuthInterceptor extends QueuedInterceptor {
  _AuthInterceptor(this._client);

  final ApiClient _client;

  static const _retriedKey = 'athlixRetried';

  @override
  Future<void> onRequest(RequestOptions options, RequestInterceptorHandler handler) async {
    if (!ApiClient._skipsAuth(options.path)) {
      final store = _client.tokens;
      if (store.hasSession &&
          store.accessExpiresWithin(const Duration(seconds: Env.refreshLeewaySeconds))) {
        await _client.refresh();
      }
      final token = store.accessToken;
      if (token != null) {
        options.headers['Authorization'] = 'Bearer $token';
      }
    }
    handler.next(options);
  }

  @override
  Future<void> onError(DioException err, ErrorInterceptorHandler handler) async {
    final opts = err.requestOptions;
    final unauthorised = err.response?.statusCode == 401;
    if (unauthorised &&
        !ApiClient._skipsAuth(opts.path) &&
        opts.extra[_retriedKey] != true &&
        _client.tokens.refreshToken != null) {
      final ok = await _client.refresh();
      final token = _client.tokens.accessToken;
      if (ok && token != null) {
        opts.extra[_retriedKey] = true;
        opts.headers['Authorization'] = 'Bearer $token';
        try {
          final res = await _client._bare.fetch<dynamic>(opts);
          handler.resolve(res);
        } on DioException catch (e) {
          if (e.response?.statusCode == 401) _client.tokens.expire();
          handler.next(e);
        }
        return;
      }
    } else if (unauthorised && !ApiClient._skipsAuth(opts.path)) {
      _client.tokens.expire();
    }
    handler.next(err);
  }
}

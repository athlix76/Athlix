import 'package:dio/dio.dart';

import '../auth/auth_interceptor.dart';
import '../auth/token_pair.dart';
import '../auth/token_refresher.dart';
import '../auth/token_store.dart';
import '../json/json_utils.dart';
import 'api_exception.dart';
import 'envelope.dart';

/// Reactions to cross-cutting errors. The session layer implements these.
class ApiListener {
  const ApiListener({
    this.onSessionExpired,
    this.onOnboardingRequired,
    this.onSuspended,
    this.onMaintenance,
  });

  /// Refresh refused or second 401: clear the session and go to login.
  final void Function()? onSessionExpired;

  /// 403 `ONBOARDING_REQUIRED` with `fields.nextStep`.
  final void Function(String? nextStep)? onOnboardingRequired;

  /// 403 `ACCOUNT_SUSPENDED`.
  final void Function()? onSuspended;

  /// 503 `MAINTENANCE`.
  final void Function()? onMaintenance;
}

/// The one HTTP client of the app. Every call goes through the envelope
/// parser and fails with a typed [ApiException]. Bodies are never logged.
class ApiClient {
  ApiClient(this.dio, {this.listener = const ApiListener()});

  final Dio dio;
  final ApiListener listener;

  /// Production wiring: auth interceptor + single-flight refresh.
  factory ApiClient.create({
    required String baseUrl,
    required TokenStore tokenStore,
    ApiListener listener = const ApiListener(),
    HttpClientAdapter? adapter,
    HttpClientAdapter? refreshAdapter,
  }) {
    BaseOptions options() => BaseOptions(
          baseUrl: baseUrl,
          connectTimeout: const Duration(seconds: 15),
          receiveTimeout: const Duration(seconds: 30),
          sendTimeout: const Duration(seconds: 30),
          contentType: Headers.jsonContentType,
          responseType: ResponseType.json,
          headers: {'Accept': 'application/json'},
        );

    final dio = Dio(options());
    if (adapter != null) dio.httpClientAdapter = adapter;

    // Bare client for /auth/refresh: no interceptor, so no recursion.
    final refreshDio = Dio(options());
    if (refreshAdapter != null) refreshDio.httpClientAdapter = refreshAdapter;

    final refresher = TokenRefresher(
      store: tokenStore,
      call: (refreshToken) async {
        try {
          final res = await refreshDio.post<dynamic>(
            '/auth/refresh',
            data: {'refreshToken': refreshToken},
          );
          final env = parseEnvelope(res.data, statusCode: res.statusCode);
          return TokenPair.fromJson(asMap(env.data));
        } on DioException catch (e) {
          throw mapDioException(e);
        }
      },
    );

    dio.interceptors.add(AuthInterceptor(
      store: tokenStore,
      refresher: refresher,
      retryDio: dio,
      onSessionExpired: () => listener.onSessionExpired?.call(),
    ));
    return ApiClient(dio, listener: listener);
  }

  // ---- verbs -------------------------------------------------------------

  Future<T> get<T>(String path, T Function(Object? data) parse,
      {Map<String, Object?>? query, bool auth = true}) async {
    final env = await send('GET', path, query: query, auth: auth);
    return parse(env.data);
  }

  Future<Paged<T>> page<T>(String path, T Function(Map<String, dynamic> json) item,
      {Map<String, Object?>? query}) async {
    final env = await send('GET', path, query: query);
    return Paged.fromEnvelope(env, item);
  }

  Future<T> post<T>(String path, T Function(Object? data) parse,
      {Object? body, bool auth = true}) async {
    final env = await send('POST', path, body: body ?? const <String, dynamic>{}, auth: auth);
    return parse(env.data);
  }

  Future<void> postVoid(String path, {Object? body, bool auth = true}) async {
    await send('POST', path, body: body ?? const <String, dynamic>{}, auth: auth);
  }

  Future<T> put<T>(String path, T Function(Object? data) parse, {Object? body}) async {
    final env = await send('PUT', path, body: body ?? const <String, dynamic>{});
    return parse(env.data);
  }

  Future<void> putVoid(String path, {Object? body}) async {
    await send('PUT', path, body: body ?? const <String, dynamic>{});
  }

  Future<T> patch<T>(String path, T Function(Object? data) parse, {Object? body}) async {
    final env = await send('PATCH', path, body: body ?? const <String, dynamic>{});
    return parse(env.data);
  }

  Future<T> deleteJson<T>(String path, T Function(Object? data) parse, {Object? body}) async {
    final env = await send('DELETE', path, body: body);
    return parse(env.data);
  }

  Future<void> delete(String path, {Object? body}) async {
    await send('DELETE', path, body: body);
  }

  /// Low-level call: returns the parsed envelope or throws [ApiException].
  Future<Envelope> send(String method, String path,
      {Map<String, Object?>? query, Object? body, bool auth = true}) async {
    try {
      final res = await dio.request<dynamic>(
        path,
        data: body,
        queryParameters: query == null ? null : compactQuery(query),
        options: Options(
          method: method,
          extra: auth ? null : {AuthInterceptor.skipAuth: true},
        ),
      );
      return parseEnvelope(
        res.data,
        statusCode: res.statusCode,
        requestId: res.headers.value('x-request-id'),
      );
    } on DioException catch (e) {
      final ex = mapDioException(e);
      _notify(ex);
      throw ex;
    } on ApiException catch (e) {
      _notify(e);
      rethrow;
    }
  }

  void _notify(ApiException e) {
    switch (e.code) {
      case ApiErrorCode.onboardingRequired:
        listener.onOnboardingRequired?.call(e.nextStep);
      case ApiErrorCode.accountSuspended:
        listener.onSuspended?.call();
      case ApiErrorCode.maintenance:
        listener.onMaintenance?.call();
      default:
        break;
    }
  }
}

/// Maps a dio failure to an [ApiException] using the error envelope when present.
ApiException mapDioException(DioException e) {
  final res = e.response;
  if (res == null) {
    switch (e.type) {
      case DioExceptionType.connectionTimeout:
      case DioExceptionType.sendTimeout:
      case DioExceptionType.receiveTimeout:
      case DioExceptionType.connectionError:
        return ApiException.network();
      default:
        return e.error is ApiException ? e.error! as ApiException : ApiException.network();
    }
  }
  return apiExceptionFromBody(
    res.data,
    statusCode: res.statusCode,
    requestId: res.headers.value('x-request-id'),
  );
}

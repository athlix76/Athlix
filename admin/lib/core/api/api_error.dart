import 'dart:convert';

import 'package:dio/dio.dart';

import 'json.dart';

/// Error codes from docs/api.md (Conventions → Envelope), plus two client-side
/// codes for failures that never reached the server.
abstract final class ErrorCodes {
  static const badRequest = 'BAD_REQUEST';
  static const unauthenticated = 'UNAUTHENTICATED';
  static const forbidden = 'FORBIDDEN';
  static const notFound = 'NOT_FOUND';

  // 403 codes the user app sees; listed so an admin screen that proxies a
  // user-facing rule (for example an eligibility check) still shows a title.
  static const onboardingRequired = 'ONBOARDING_REQUIRED';
  static const verificationRequired = 'VERIFICATION_REQUIRED';
  static const organiserNotApproved = 'ORGANISER_NOT_APPROVED';
  static const planRequired = 'PLAN_REQUIRED';
  static const accountSuspended = 'ACCOUNT_SUSPENDED';
  static const notEligible = 'NOT_ELIGIBLE';
  static const conflict = 'CONFLICT';
  static const invalidState = 'INVALID_STATE';
  static const limitReached = 'LIMIT_REACHED';
  static const validationFailed = 'VALIDATION_FAILED';
  static const rateLimited = 'RATE_LIMITED';
  static const upstreamError = 'UPSTREAM_ERROR';
  static const maintenance = 'MAINTENANCE';
  static const internal = 'INTERNAL';

  /// Client only: no response (offline, DNS, CORS refused).
  static const network = 'NETWORK';

  /// Client only: the request timed out.
  static const timeout = 'TIMEOUT';
}

/// A failed API call in one shape. Screens switch on [code], never on [message].
class ApiError implements Exception {
  const ApiError({
    required this.code,
    required this.message,
    this.status,
    this.fields = const {},
    this.requestId,
  });

  final int? status;
  final String code;

  /// Safe to show (the backend promises this; client messages are generic).
  final String message;

  /// Per-field messages for `VALIDATION_FAILED`, or extra details such as
  /// `benefit` / `nextStep` for other codes.
  final Map<String, String> fields;
  final String? requestId;

  bool get isForbidden => status == 403 || code == ErrorCodes.forbidden;
  bool get isNotFound => code == ErrorCodes.notFound;
  bool get isUnauthenticated => status == 401 || code == ErrorCodes.unauthenticated;

  /// Short heading for the error banner.
  String get title => _titles[code] ?? code;

  /// What the admin can do about it, or null.
  String? get hint => _hints[code];

  static const _titles = <String, String>{
    ErrorCodes.badRequest: 'Bad request',
    ErrorCodes.unauthenticated: 'Signed out',
    ErrorCodes.forbidden: 'Not allowed',
    ErrorCodes.notFound: 'Not found',
    ErrorCodes.onboardingRequired: 'Onboarding not finished',
    ErrorCodes.verificationRequired: 'Verification required',
    ErrorCodes.organiserNotApproved: 'Organiser not approved',
    ErrorCodes.planRequired: 'Premium plan required',
    ErrorCodes.accountSuspended: 'Account suspended',
    ErrorCodes.notEligible: 'Not eligible (location)',
    ErrorCodes.conflict: 'Conflict',
    ErrorCodes.invalidState: 'Not allowed in this state',
    ErrorCodes.limitReached: 'Limit reached',
    ErrorCodes.validationFailed: 'Check the fields',
    ErrorCodes.rateLimited: 'Too many requests',
    ErrorCodes.upstreamError: 'Provider error',
    ErrorCodes.maintenance: 'Maintenance',
    ErrorCodes.internal: 'Server error',
    ErrorCodes.network: 'Cannot reach the server',
    ErrorCodes.timeout: 'Timed out',
  };

  static const _hints = <String, String>{
    ErrorCodes.unauthenticated: 'Sign in again.',
    ErrorCodes.forbidden: 'Your role does not include this permission. Ask a super admin.',
    ErrorCodes.notFound: 'It may have been removed. Reload the list.',
    ErrorCodes.conflict: 'Someone changed this first, or it already exists. Reload and try again.',
    ErrorCodes.invalidState: 'Reload to see the current status.',
    ErrorCodes.validationFailed: 'Fix the fields listed below and try again.',
    ErrorCodes.rateLimited: 'Wait a minute and try again.',
    ErrorCodes.upstreamError: 'The payment / store / verification provider failed. Try again later.',
    ErrorCodes.internal: 'Try again. If it keeps failing, report the request id.',
    ErrorCodes.network: 'Check your connection and that the API allows this origin.',
    ErrorCodes.timeout: 'Try again.',
  };

  static String _codeForStatus(int? status) {
    switch (status) {
      case 400:
        return ErrorCodes.badRequest;
      case 401:
        return ErrorCodes.unauthenticated;
      case 403:
        return ErrorCodes.forbidden;
      case 404:
        return ErrorCodes.notFound;
      case 409:
        return ErrorCodes.conflict;
      case 422:
        return ErrorCodes.validationFailed;
      case 429:
        return ErrorCodes.rateLimited;
      case 502:
        return ErrorCodes.upstreamError;
      case 503:
        return ErrorCodes.maintenance;
      default:
        return ErrorCodes.internal;
    }
  }

  /// Builds an error from an HTTP status and a response body (decoded or raw).
  factory ApiError.fromResponse(int? status, Object? body, {String? requestId}) {
    Object? decoded = body;
    if (body is String && body.isNotEmpty) {
      try {
        decoded = jsonDecode(body);
      } on FormatException {
        decoded = null;
      }
    }
    final err = decoded is Map ? asJsonOrNull(decoded['error']) : null;
    if (err != null && asString(err['code']).isNotEmpty) {
      final fields = <String, String>{};
      asJson(err['fields']).forEach((k, v) => fields[k] = asString(v));
      return ApiError(
        status: status,
        code: asString(err['code']),
        message: asString(err['message'], 'Request failed.'),
        fields: fields,
        requestId: requestId,
      );
    }
    final code = _codeForStatus(status);
    return ApiError(
      status: status,
      code: code,
      message: 'Request failed (HTTP ${status ?? '?'}).',
      requestId: requestId,
    );
  }

  factory ApiError.fromDio(DioException e) {
    final res = e.response;
    if (res == null) {
      switch (e.type) {
        case DioExceptionType.connectionTimeout:
        case DioExceptionType.sendTimeout:
        case DioExceptionType.receiveTimeout:
          return const ApiError(code: ErrorCodes.timeout, message: 'The server took too long to answer.');
        default:
          return const ApiError(code: ErrorCodes.network, message: 'The server could not be reached.');
      }
    }
    return ApiError.fromResponse(
      res.statusCode,
      res.data,
      requestId: res.headers.value('x-request-id'),
    );
  }

  /// A client-side form check that failed before any request was sent.
  factory ApiError.invalidInput(Map<String, String> fields) => ApiError(
        code: ErrorCodes.validationFailed,
        message: 'Some fields need attention (checked before sending).',
        fields: fields,
      );

  /// Normalises anything thrown by the data layer.
  factory ApiError.from(Object error) {
    if (error is ApiError) return error;
    if (error is DioException) return ApiError.fromDio(error);
    return const ApiError(code: ErrorCodes.internal, message: 'Something went wrong.');
  }

  @override
  String toString() => 'ApiError($code, status: $status)';
}

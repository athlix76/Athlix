/// Error codes from docs/api.md ("Envelope"). Clients switch on the code,
/// never on the message.
enum ApiErrorCode {
  badRequest('BAD_REQUEST'),
  unauthenticated('UNAUTHENTICATED'),
  forbidden('FORBIDDEN'),
  onboardingRequired('ONBOARDING_REQUIRED'),
  verificationRequired('VERIFICATION_REQUIRED'),
  organiserNotApproved('ORGANISER_NOT_APPROVED'),
  planRequired('PLAN_REQUIRED'),
  accountSuspended('ACCOUNT_SUSPENDED'),

  /// Location eligibility not met; `fields.reason` / `fields.action`.
  notEligible('NOT_ELIGIBLE'),
  notFound('NOT_FOUND'),
  conflict('CONFLICT'),
  invalidState('INVALID_STATE'),
  limitReached('LIMIT_REACHED'),
  validationFailed('VALIDATION_FAILED'),
  rateLimited('RATE_LIMITED'),
  upstreamError('UPSTREAM_ERROR'),
  maintenance('MAINTENANCE'),
  internal('INTERNAL'),

  /// Client side: no response (offline, timeout, DNS).
  network('NETWORK'),

  /// Client side: a response we could not understand.
  unknown('UNKNOWN');

  const ApiErrorCode(this.wire);

  /// The string the API sends in `error.code`.
  final String wire;

  static ApiErrorCode fromWire(String? code) {
    for (final c in ApiErrorCode.values) {
      if (c.wire == code) return c;
    }
    return ApiErrorCode.unknown;
  }

  /// Fallback when the body has no error object: map by HTTP status.
  static ApiErrorCode fromStatus(int? status) {
    switch (status) {
      case 400:
        return ApiErrorCode.badRequest;
      case 401:
        return ApiErrorCode.unauthenticated;
      case 403:
        return ApiErrorCode.forbidden;
      case 404:
        return ApiErrorCode.notFound;
      case 409:
        return ApiErrorCode.conflict;
      case 422:
        return ApiErrorCode.validationFailed;
      case 429:
        return ApiErrorCode.rateLimited;
      case 502:
        return ApiErrorCode.upstreamError;
      case 503:
        return ApiErrorCode.maintenance;
      case null:
        return ApiErrorCode.network;
      default:
        return status >= 500 ? ApiErrorCode.internal : ApiErrorCode.unknown;
    }
  }
}

/// A failed API call. [message] is safe to show to users (the API guarantees
/// it); client-side codes get a generic message.
class ApiException implements Exception {
  const ApiException({
    required this.code,
    required this.message,
    this.fields = const {},
    this.statusCode,
    this.requestId,
  });

  final ApiErrorCode code;
  final String message;

  /// Per-field validation messages, or extra keys such as `nextStep` / `benefit`.
  final Map<String, String> fields;
  final int? statusCode;

  /// `X-Request-Id` of the response, for bug reports. Not a secret.
  final String? requestId;

  factory ApiException.network() => const ApiException(
        code: ApiErrorCode.network,
        message: 'No connection. Check your internet and try again.',
      );

  factory ApiException.unknown([int? status]) => ApiException(
        code: ApiErrorCode.unknown,
        message: 'Something went wrong. Please try again.',
        statusCode: status,
      );

  /// `ONBOARDING_REQUIRED` → `role | verification | guardian | profile`.
  String? get nextStep => fields['nextStep'];

  /// `PLAN_REQUIRED` → the premium benefit key that is missing.
  String? get benefit => fields['benefit'];

  bool get isNotFound => code == ApiErrorCode.notFound;
  bool get isValidation => code == ApiErrorCode.validationFailed;

  @override
  String toString() =>
      'ApiException(${code.wire}, status: $statusCode, requestId: $requestId)';
}

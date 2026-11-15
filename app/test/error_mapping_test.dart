import 'package:athlix/core/api/api_client.dart';
import 'package:athlix/core/api/api_exception.dart';
import 'package:athlix/core/widgets/error_view.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('ApiErrorCode.fromWire', () {
    test('every code round-trips through its wire value', () {
      for (final c in ApiErrorCode.values) {
        expect(ApiErrorCode.fromWire(c.wire), c);
      }
    });

    test('unknown and null map to unknown', () {
      expect(ApiErrorCode.fromWire('NOPE'), ApiErrorCode.unknown);
      expect(ApiErrorCode.fromWire(null), ApiErrorCode.unknown);
    });

    test('location eligibility code exists', () {
      expect(ApiErrorCode.fromWire('NOT_ELIGIBLE'), ApiErrorCode.notEligible);
    });
  });

  group('ApiErrorCode.fromStatus', () {
    final cases = <int?, ApiErrorCode>{
      400: ApiErrorCode.badRequest,
      401: ApiErrorCode.unauthenticated,
      403: ApiErrorCode.forbidden,
      404: ApiErrorCode.notFound,
      409: ApiErrorCode.conflict,
      422: ApiErrorCode.validationFailed,
      429: ApiErrorCode.rateLimited,
      500: ApiErrorCode.internal,
      502: ApiErrorCode.upstreamError,
      503: ApiErrorCode.maintenance,
      504: ApiErrorCode.internal,
      418: ApiErrorCode.unknown,
      null: ApiErrorCode.network,
    };
    cases.forEach((status, code) {
      test('$status -> ${code.wire}', () => expect(ApiErrorCode.fromStatus(status), code));
    });
  });

  group('ApiException helpers', () {
    test('nextStep and benefit read fields', () {
      const e = ApiException(
        code: ApiErrorCode.onboardingRequired,
        message: 'Finish onboarding',
        fields: {'nextStep': 'guardian', 'benefit': 'advanced_search'},
      );
      expect(e.nextStep, 'guardian');
      expect(e.benefit, 'advanced_search');
    });
  });

  group('errorMessage', () {
    test('API messages are shown as is', () {
      expect(errorMessage(const ApiException(code: ApiErrorCode.notFound, message: 'Event not found')),
          'Event not found');
    });

    test('validation errors list their fields', () {
      final text = errorMessage(const ApiException(
        code: ApiErrorCode.validationFailed,
        message: 'Check the form.',
        fields: {'title': 'Required'},
      ));
      expect(text, contains('Check the form.'));
      expect(text, contains('title: Required'));
    });

    test('NOT_ELIGIBLE shows the message, not the reason/action keys', () {
      final text = errorMessage(const ApiException(
        code: ApiErrorCode.notEligible,
        message: 'Only for players from Thanjavur.',
        fields: {'reason': 'wrong_location', 'action': 'none'},
      ));
      expect(text, 'Only for players from Thanjavur.');
    });

    test('non-API errors never expose toString', () {
      final text = errorMessage(StateError('secret-ish internals'));
      expect(text, isNot(contains('internals')));
    });
  });

  group('mapDioException', () {
    final options = RequestOptions(path: '/x');

    test('timeouts and connection errors are network', () {
      for (final t in [
        DioExceptionType.connectionTimeout,
        DioExceptionType.receiveTimeout,
        DioExceptionType.sendTimeout,
        DioExceptionType.connectionError,
      ]) {
        expect(mapDioException(DioException(requestOptions: options, type: t)).code,
            ApiErrorCode.network);
      }
    });

    test('a response with an error envelope keeps the code and request id', () {
      final e = mapDioException(DioException(
        requestOptions: options,
        type: DioExceptionType.badResponse,
        response: Response(
          requestOptions: options,
          statusCode: 403,
          data: {
            'error': {
              'code': 'NOT_ELIGIBLE',
              'message': 'Not eligible',
              'fields': {'reason': 'identity_missing', 'action': 'verify_native'},
            }
          },
          headers: Headers.fromMap({
            'x-request-id': ['r-9']
          }),
        ),
      ));
      expect(e.code, ApiErrorCode.notEligible);
      expect(e.fields['action'], 'verify_native');
      expect(e.requestId, 'r-9');
      expect(e.statusCode, 403);
    });
  });
}

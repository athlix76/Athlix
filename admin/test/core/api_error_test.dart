import 'package:athlix_admin/core/api/api_error.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('ApiError.fromResponse', () {
    test('reads the error envelope', () {
      final e = ApiError.fromResponse(422, {
        'error': {
          'code': 'VALIDATION_FAILED',
          'message': 'Check the fields',
          'fields': {'value': 'must be >= 1', 'n': 3},
        },
      }, requestId: 'req-1');
      expect(e.status, 422);
      expect(e.code, ErrorCodes.validationFailed);
      expect(e.message, 'Check the fields');
      expect(e.fields, {'value': 'must be >= 1', 'n': '3'});
      expect(e.requestId, 'req-1');
      expect(e.title, 'Check the fields');
      expect(e.hint, isNotNull);
    });

    test('decodes a raw JSON string body', () {
      final e = ApiError.fromResponse(409, '{"error":{"code":"CONFLICT","message":"stale version"}}');
      expect(e.code, ErrorCodes.conflict);
      expect(e.message, 'stale version');
      expect(e.fields, isEmpty);
    });

    test('keeps unknown backend codes and uses the code as the title', () {
      final e = ApiError.fromResponse(400, {
        'error': {'code': 'SOMETHING_NEW', 'message': 'x'},
      });
      expect(e.code, 'SOMETHING_NEW');
      expect(e.title, 'SOMETHING_NEW');
      expect(e.hint, isNull);
    });

    test('maps the HTTP status when there is no envelope', () {
      final cases = {
        400: ErrorCodes.badRequest,
        401: ErrorCodes.unauthenticated,
        403: ErrorCodes.forbidden,
        404: ErrorCodes.notFound,
        409: ErrorCodes.conflict,
        422: ErrorCodes.validationFailed,
        429: ErrorCodes.rateLimited,
        502: ErrorCodes.upstreamError,
        503: ErrorCodes.maintenance,
        500: ErrorCodes.internal,
        null: ErrorCodes.internal,
      };
      cases.forEach((status, code) {
        expect(ApiError.fromResponse(status, '<html>gateway</html>').code, code, reason: '$status');
      });
    });

    test('never copies a non-envelope body into the message', () {
      final e = ApiError.fromResponse(500, 'stack trace with secrets');
      expect(e.message, isNot(contains('secrets')));
    });

    test('403 and 401 helpers', () {
      expect(ApiError.fromResponse(403, null).isForbidden, isTrue);
      expect(ApiError.fromResponse(401, null).isUnauthenticated, isTrue);
      final nf = ApiError.fromResponse(404, {
        'error': {'code': 'NOT_FOUND', 'message': 'gone'},
      });
      expect(nf.isNotFound, isTrue);
    });
  });

  group('ApiError.fromDio', () {
    final opts = RequestOptions(path: '/admin/config');

    test('timeout without a response', () {
      final e = ApiError.fromDio(DioException(requestOptions: opts, type: DioExceptionType.receiveTimeout));
      expect(e.code, ErrorCodes.timeout);
      expect(e.status, isNull);
    });

    test('no response (network or CORS)', () {
      final e = ApiError.fromDio(DioException(requestOptions: opts, type: DioExceptionType.connectionError));
      expect(e.code, ErrorCodes.network);
    });

    test('response with envelope and request id header', () {
      final e = ApiError.fromDio(DioException(
        requestOptions: opts,
        type: DioExceptionType.badResponse,
        response: Response<dynamic>(
          requestOptions: opts,
          statusCode: 409,
          data: {
            'error': {'code': 'CONFLICT', 'message': 'version changed'},
          },
          headers: Headers.fromMap({
            'x-request-id': ['abc-123'],
          }),
        ),
      ));
      expect(e.status, 409);
      expect(e.code, ErrorCodes.conflict);
      expect(e.requestId, 'abc-123');
    });
  });

  group('ApiError.from / invalidInput', () {
    test('passes ApiError through and hides unknown errors', () {
      const original = ApiError(code: ErrorCodes.notFound, message: 'x');
      expect(identical(ApiError.from(original), original), isTrue);
      final e = ApiError.from(StateError('internal detail'));
      expect(e.code, ErrorCodes.internal);
      expect(e.message, isNot(contains('internal detail')));
    });

    test('client-side validation looks like a 422', () {
      final e = ApiError.invalidInput({'aadhaarLast4': 'Exactly 4 digits'});
      expect(e.code, ErrorCodes.validationFailed);
      expect(e.fields['aadhaarLast4'], 'Exactly 4 digits');
      expect(e.status, isNull);
    });
  });
}

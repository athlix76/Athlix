import 'package:athlix/core/api/api_exception.dart';
import 'package:athlix/core/api/envelope.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('parseEnvelope', () {
    test('success with data and meta', () {
      final env = parseEnvelope({
        'data': [
          {'id': 1}
        ],
        'meta': {'nextCursor': 'abc', 'unreadCount': 3, 'total': 10},
      }, statusCode: 200);
      expect(env.data, isA<List>());
      expect(env.meta.nextCursor, 'abc');
      expect(env.meta.unreadCount, 3);
      expect(env.meta.total, 10);
    });

    test('empty next cursor means last page', () {
      final env = parseEnvelope({'data': [], 'meta': {'nextCursor': ''}}, statusCode: 200);
      expect(env.meta.nextCursor, isNull);
    });

    test('204 / empty body gives an empty envelope', () {
      expect(parseEnvelope(null, statusCode: 204).data, isNull);
      expect(parseEnvelope('', statusCode: 204).data, isNull);
    });

    test('error body throws with code, message and string fields', () {
      expect(
        () => parseEnvelope({
          'error': {
            'code': 'VALIDATION_FAILED',
            'message': 'Check the form.',
            'fields': {'title': 'Required', 'maxSlots': 5},
          }
        }, statusCode: 422, requestId: 'req-1'),
        throwsA(isA<ApiException>()
            .having((e) => e.code, 'code', ApiErrorCode.validationFailed)
            .having((e) => e.message, 'message', 'Check the form.')
            .having((e) => e.fields, 'fields', {'title': 'Required', 'maxSlots': '5'})
            .having((e) => e.statusCode, 'status', 422)
            .having((e) => e.requestId, 'requestId', 'req-1')),
      );
    });

    test('error body wins even with a 200 status', () {
      expect(
        () => parseEnvelope({'error': {'code': 'NOT_FOUND', 'message': 'Gone'}}, statusCode: 200),
        throwsA(isA<ApiException>().having((e) => e.code, 'code', ApiErrorCode.notFound)),
      );
    });

    test('non-2xx without an error object maps by status', () {
      expect(
        () => parseEnvelope('<html>bad gateway</html>', statusCode: 502),
        throwsA(isA<ApiException>().having((e) => e.code, 'code', ApiErrorCode.upstreamError)),
      );
    });

    test('a map without data is unknown', () {
      expect(
        () => parseEnvelope({'foo': 1}, statusCode: 200),
        throwsA(isA<ApiException>().having((e) => e.code, 'code', ApiErrorCode.unknown)),
      );
    });

    test('a non-map body on 2xx is unknown', () {
      expect(
        () => parseEnvelope([1, 2], statusCode: 200),
        throwsA(isA<ApiException>().having((e) => e.code, 'code', ApiErrorCode.unknown)),
      );
    });
  });

  group('apiExceptionFromBody', () {
    test('missing message falls back to a generic sentence', () {
      final e = apiExceptionFromBody({'error': {'code': 'INTERNAL'}}, statusCode: 500);
      expect(e.code, ApiErrorCode.internal);
      expect(e.message, isNotEmpty);
    });

    test('unknown wire code maps to unknown', () {
      final e = apiExceptionFromBody({'error': {'code': 'SOMETHING_NEW', 'message': 'x'}});
      expect(e.code, ApiErrorCode.unknown);
    });

    test('null fields are dropped', () {
      final e = apiExceptionFromBody({
        'error': {'code': 'BAD_REQUEST', 'message': 'x', 'fields': {'a': null, 'b': 'y'}}
      });
      expect(e.fields, {'b': 'y'});
    });

    test('toString never includes the message or fields', () {
      final e = apiExceptionFromBody({
        'error': {'code': 'BAD_REQUEST', 'message': 'mobile 9876543210 invalid', 'fields': {'mobile': '9876543210'}}
      }, statusCode: 400);
      expect(e.toString(), isNot(contains('9876543210')));
    });
  });

  group('Paged', () {
    test('builds items and cursor from an envelope', () {
      final env = parseEnvelope({
        'data': [
          {'id': 1},
          {'id': 2}
        ],
        'meta': {'nextCursor': 'n2'},
      }, statusCode: 200);
      final page = Paged.fromEnvelope(env, (m) => m['id'] as int);
      expect(page.items, [1, 2]);
      expect(page.hasMore, isTrue);
    });

    test('no cursor is the last page', () {
      final page = Paged.fromEnvelope(parseEnvelope({'data': []}, statusCode: 200), (m) => m);
      expect(page.items, isEmpty);
      expect(page.hasMore, isFalse);
    });
  });
}

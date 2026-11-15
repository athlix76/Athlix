import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';

/// `targetType` values of `POST /reports`.
abstract final class ReportTarget {
  static const user = 'user';
  static const event = 'event';
  static const post = 'post';
  static const comment = 'comment';
  static const message = 'message';
  static const conversation = 'conversation';
  static const community = 'community';
}

/// `reason` values of `POST /reports` (contract list, in display order).
abstract final class ReportReason {
  static const spam = 'spam';
  static const fakeProfile = 'fake_profile';
  static const harassment = 'harassment';
  static const scam = 'scam';
  static const inappropriate = 'inappropriate';
  static const underageSafety = 'underage_safety';
  static const paymentIssue = 'payment_issue';
  static const other = 'other';

  static const all = [spam, fakeProfile, harassment, scam, inappropriate, underageSafety, paymentIssue, other];

  static const labels = {
    spam: 'Spam',
    fakeProfile: 'Fake profile',
    harassment: 'Harassment or bullying',
    scam: 'Scam or fraud',
    inappropriate: 'Inappropriate content',
    underageSafety: 'Safety of a minor',
    paymentIssue: 'Payment issue',
    other: 'Something else',
  };
}

class ReportApi {
  const ReportApi(this._api);

  final ApiClient _api;

  /// Returns the report id. Reporting the same target twice returns the existing id.
  Future<int> create({required String targetType, required int targetId, required String reason, String? details}) =>
      _api.post(
        '/reports',
        (d) => asInt(asMap(d)['id']),
        body: {
          'targetType': targetType,
          'targetId': targetId,
          'reason': reason,
          if (details != null && details.isNotEmpty) 'details': details,
        },
      );
}

final reportApiProvider = Provider<ReportApi>((ref) => ReportApi(ref.watch(apiClientProvider)));

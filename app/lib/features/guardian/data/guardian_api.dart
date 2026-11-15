import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/models/me.dart';
import '../../../core/providers.dart';

const guardianRelations = ['mother', 'father', 'legal_guardian', 'other'];

/// Body of `POST /onboarding/guardian/request`.
class GuardianRequest {
  const GuardianRequest({
    required this.guardianName,
    required this.guardianMobile,
    required this.relation,
    required this.policyDocumentId,
  });

  final String guardianName;

  /// Full number, sent once to the server; never logged or stored on device.
  final String guardianMobile;
  final String relation;
  final int policyDocumentId;

  Map<String, dynamic> toJson() => {
        'guardianName': guardianName,
        'guardianMobile': guardianMobile,
        'relation': relation,
        'policyDocumentId': policyDocumentId,
      };

  @override
  String toString() => 'GuardianRequest(<redacted>)';
}

/// Response of the request call. [consentId] is opaque and sent back as received.
class GuardianChallenge {
  const GuardianChallenge({
    required this.consentId,
    required this.guardianMobileMasked,
    required this.expiresInSeconds,
    required this.resendAfterSeconds,
  });

  final Object consentId;
  final String guardianMobileMasked;
  final int expiresInSeconds;
  final int resendAfterSeconds;

  factory GuardianChallenge.fromJson(Map<String, dynamic> json) => GuardianChallenge(
        consentId: json['consentId'] ?? '',
        guardianMobileMasked: asString(json['guardianMobileMasked']),
        expiresInSeconds: asInt(json['expiresInSeconds']),
        resendAfterSeconds: asInt(json['resendAfterSeconds']),
      );
}

class GuardianApi {
  const GuardianApi(this._api);

  final ApiClient _api;

  Future<GuardianChallenge> request(GuardianRequest req) => _api.post(
        '/onboarding/guardian/request',
        (d) => GuardianChallenge.fromJson(asMap(d)),
        body: req.toJson(),
      );

  Future<Me> verify(Object consentId, String otp) => _api.post(
        '/onboarding/guardian/verify',
        (d) => Me.fromJson(asMap(d)),
        body: {'consentId': consentId, 'otp': otp},
      );

  /// `POST /me/guardian/withdraw/request`: sends a code to the guardian who
  /// gave consent. Only for a minor whose consent is `granted`.
  Future<GuardianChallenge> withdrawRequest() => _api.post(
        '/me/guardian/withdraw/request',
        (d) => GuardianChallenge.fromJson(asMap(d)),
      );

  /// `POST /me/guardian/withdraw/verify`: consent becomes `revoked` and the
  /// user is sent back to the guardian onboarding step.
  Future<Me> withdrawVerify(Object consentId, String otp) => _api.post(
        '/me/guardian/withdraw/verify',
        (d) => Me.fromJson(asMap(d)),
        body: {'consentId': consentId, 'otp': otp},
      );
}

final guardianApiProvider = Provider<GuardianApi>((ref) => GuardianApi(ref.watch(apiClientProvider)));

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';

/// `VerificationStatus`.
class VerificationStatus {
  const VerificationStatus({
    required this.status,
    this.provider,
    this.aadhaarLast4,
    this.verifiedAt,
    this.failureReason,
    required this.isMinor,
    required this.attemptsLeftToday,
  });

  /// `unverified | pending | verified | failed`.
  final String status;
  final String? provider;
  final String? aadhaarLast4;
  final DateTime? verifiedAt;

  /// `too_young | already_linked | provider_error | cancelled | name_mismatch`.
  final String? failureReason;
  final bool isMinor;
  final int attemptsLeftToday;

  bool get isVerified => status == 'verified';

  factory VerificationStatus.fromJson(Map<String, dynamic> json) => VerificationStatus(
        status: asString(json['status'], 'unverified'),
        provider: asStringOrNull(json['provider']),
        aadhaarLast4: asStringOrNull(json['aadhaarLast4']),
        verifiedAt: asDateTimeOrNull(json['verifiedAt']),
        failureReason: asStringOrNull(json['failureReason']),
        isMinor: asBool(json['isMinor']),
        attemptsLeftToday: asInt(json['attemptsLeftToday']),
      );
}

/// `POST /verification/digilocker/start` response.
class DigilockerStart {
  const DigilockerStart({required this.authorizationUrl, this.expiresAt});

  final String authorizationUrl;
  final DateTime? expiresAt;

  factory DigilockerStart.fromJson(Map<String, dynamic> json) => DigilockerStart(
        authorizationUrl: asString(json['authorizationUrl']),
        expiresAt: asDateTimeOrNull(json['expiresAt']),
      );
}

/// Outcomes accepted by the non-prod mock provider.
const mockVerificationOutcomes = ['adult', 'minor', 'too_young', 'duplicate', 'failed'];

class VerificationApi {
  const VerificationApi(this._api);

  final ApiClient _api;

  Future<DigilockerStart> start() => _api.post(
        '/verification/digilocker/start',
        (d) => DigilockerStart.fromJson(asMap(d)),
      );

  Future<VerificationStatus> status() =>
      _api.get('/verification/status', (d) => VerificationStatus.fromJson(asMap(d)));

  /// Only works when the server runs the mock provider outside prod (else 404).
  /// Non-production only. [nativeDistrictId] simulates the DigiLocker
  /// address district so a verified native identity is created.
  Future<VerificationStatus> mockComplete(String outcome, {int? nativeDistrictId}) => _api.post(
        '/verification/mock/complete',
        (d) => VerificationStatus.fromJson(asMap(d)),
        body: {'outcome': outcome, if (nativeDistrictId != null) 'nativeDistrictId': nativeDistrictId},
      );
}

final verificationApiProvider =
    Provider<VerificationApi>((ref) => VerificationApi(ref.watch(apiClientProvider)));

/// User-facing text for `failureReason` / the deep link `reason`.
String verificationFailureText(String? reason) {
  switch (reason) {
    case 'too_young':
      return 'You are below the minimum age to use Athlix.';
    case 'already_linked':
      return 'This DigiLocker identity is already linked to another Athlix account.';
    case 'cancelled':
      return 'Verification was cancelled.';
    case 'name_mismatch':
      return 'The name on DigiLocker does not match your profile name.';
    case 'provider_error':
      return 'DigiLocker could not be reached. Please try again.';
    default:
      return 'Verification failed. Please try again.';
  }
}

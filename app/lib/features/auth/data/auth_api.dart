import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/auth/token_pair.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/models/me.dart';
import '../../../core/providers.dart';

/// `POST /auth/otp/request` response. [requestId] is opaque: it is sent back
/// exactly as received (the contract does not fix its JSON type).
class OtpChallenge {
  const OtpChallenge({
    required this.requestId,
    required this.resendAfterSeconds,
    required this.expiresInSeconds,
  });

  final Object requestId;
  final int resendAfterSeconds;
  final int expiresInSeconds;

  factory OtpChallenge.fromJson(Map<String, dynamic> json) => OtpChallenge(
        requestId: json['requestId'] ?? '',
        resendAfterSeconds: asInt(json['resendAfterSeconds']),
        expiresInSeconds: asInt(json['expiresInSeconds']),
      );
}

/// `POST /auth/otp/verify` response.
class AuthResult {
  const AuthResult(
      {required this.tokens, required this.user, required this.isNewUser});

  final TokenPair tokens;
  final Me user;
  final bool isNewUser;

  factory AuthResult.fromJson(Map<String, dynamic> json) => AuthResult(
        tokens: TokenPair.fromJson(asMap(json['tokens'])),
        user: Me.fromJson(asMap(json['user'])),
        isNewUser: asBool(json['isNewUser']),
      );

  @override
  String toString() => 'AuthResult(<redacted>)';
}

class AuthApi {
  const AuthApi(this._api);

  final ApiClient _api;

  Future<OtpChallenge> requestOtp(String mobile) => _api.post(
        '/auth/otp/request',
        (d) => OtpChallenge.fromJson(asMap(d)),
        body: {'mobile': mobile},
        auth: false,
      );

  /// No `deviceId` is sent: the app does not collect device identifiers.
  Future<AuthResult> verifyOtp(Object requestId, String otp) => _api.post(
        '/auth/otp/verify',
        (d) => AuthResult.fromJson(asMap(d)),
        body: {'requestId': requestId, 'otp': otp},
        auth: false,
      );
}

final authApiProvider =
    Provider<AuthApi>((ref) => AuthApi(ref.watch(apiClientProvider)));

/// Client-side shape check only (the server normalises and validates):
/// 10 digits with an optional `+91` / `91` prefix.
String? normaliseMobileInput(String input) {
  final digits = input.replaceAll(RegExp(r'[\s\-()]'), '');
  final m = RegExp(r'^(?:\+?91)?(\d{10})$').firstMatch(digits);
  return m?.group(1);
}

/// `******3210` for display (never show or log the full number).
String maskMobile(String tenDigits) => tenDigits.length < 4
    ? '****'
    : '******${tenDigits.substring(tenDigits.length - 4)}';

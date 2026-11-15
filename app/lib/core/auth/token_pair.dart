import '../json/json_utils.dart';

/// `TokenPair` from docs/api.md. Never log, print or send this anywhere except
/// the `Authorization` header and `POST /auth/refresh` / `POST /auth/logout`.
class TokenPair {
  const TokenPair({
    required this.accessToken,
    required this.accessTokenExpiresAt,
    required this.refreshToken,
    required this.refreshTokenExpiresAt,
  });

  final String accessToken;
  final DateTime? accessTokenExpiresAt;
  final String refreshToken;
  final DateTime? refreshTokenExpiresAt;

  factory TokenPair.fromJson(Map<String, dynamic> json) => TokenPair(
        accessToken: asString(json['accessToken']),
        accessTokenExpiresAt: asDateTimeOrNull(json['accessTokenExpiresAt']),
        refreshToken: asString(json['refreshToken']),
        refreshTokenExpiresAt: asDateTimeOrNull(json['refreshTokenExpiresAt']),
      );

  Map<String, dynamic> toJson() => {
        'accessToken': accessToken,
        'accessTokenExpiresAt': formatRfc3339OrNull(accessTokenExpiresAt),
        'refreshToken': refreshToken,
        'refreshTokenExpiresAt': formatRfc3339OrNull(refreshTokenExpiresAt),
      };

  bool get isValid => accessToken.isNotEmpty && refreshToken.isNotEmpty;

  /// Redacted on purpose: a stray `print(tokens)` must not leak them.
  @override
  String toString() => 'TokenPair(<redacted>)';
}

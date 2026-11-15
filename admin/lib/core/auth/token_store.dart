import '../api/json.dart';

/// `TokenPair` from docs/api.md.
class TokenPair {
  const TokenPair({
    required this.accessToken,
    required this.accessTokenExpiresAt,
    required this.refreshToken,
    required this.refreshTokenExpiresAt,
  });

  final String accessToken;
  final DateTime accessTokenExpiresAt;
  final String refreshToken;
  final DateTime refreshTokenExpiresAt;

  factory TokenPair.fromJson(Json j) => TokenPair(
        accessToken: asString(j['accessToken']),
        accessTokenExpiresAt: asDate(j['accessTokenExpiresAt']) ?? DateTime.now().toUtc(),
        refreshToken: asString(j['refreshToken']),
        refreshTokenExpiresAt: asDate(j['refreshTokenExpiresAt']) ?? DateTime.now().toUtc(),
      );

  /// Never print token values.
  @override
  String toString() => 'TokenPair(<redacted>)';
}

/// Holds the admin tokens **in memory only**: no localStorage, no
/// sessionStorage, no cookies. Closing or reloading the tab ends the session,
/// which is intended for an admin tool.
class TokenStore {
  TokenPair? _pair;

  /// Called when the session ended without the admin asking (refresh failed).
  void Function()? onExpired;

  /// Called after a successful refresh (permissions may have changed).
  void Function()? onRefreshed;

  bool get hasSession => _pair != null;
  String? get accessToken => _pair?.accessToken;
  String? get refreshToken => _pair?.refreshToken;

  bool accessExpiresWithin(Duration d, {DateTime? now}) {
    final p = _pair;
    if (p == null) return false;
    return !(now ?? DateTime.now().toUtc()).add(d).isBefore(p.accessTokenExpiresAt);
  }

  bool refreshExpired({DateTime? now}) {
    final p = _pair;
    if (p == null) return true;
    return !(now ?? DateTime.now().toUtc()).isBefore(p.refreshTokenExpiresAt);
  }

  void set(TokenPair pair) => _pair = pair;

  /// Drops the tokens (sign-out the admin asked for).
  void clear() => _pair = null;

  /// Drops the tokens and tells the session (refresh failed / revoked).
  void expire() {
    final had = _pair != null;
    _pair = null;
    if (had) onExpired?.call();
  }
}

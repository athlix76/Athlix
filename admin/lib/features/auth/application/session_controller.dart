import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/auth/admin_me.dart';
import '../../../core/auth/permissions.dart';
import '../../../core/auth/token_store.dart';
import '../../../core/providers.dart';
import '../data/auth_api.dart';

/// The signed-in admin. Tokens stay in [TokenStore] (memory only); this holds
/// the profile and permissions that drive the menu and the route gates.
class SessionController extends ChangeNotifier {
  SessionController(this._api, this._tokens) {
    _tokens.onExpired = _onExpired;
    _tokens.onRefreshed = _onRefreshed;
  }

  final AuthApi _api;
  final TokenStore _tokens;

  AdminMe? _me;
  String? _notice;
  bool _signingOut = false;

  AdminMe? get me => _me;
  bool get isSignedIn => _me != null && _tokens.hasSession;
  List<String> get permissions => _me?.permissions ?? const [];

  /// One-time message for the login screen ("session ended", "signed out after inactivity").
  String? takeNotice() {
    final n = _notice;
    _notice = null;
    return n;
  }

  String? get notice => _notice;

  bool can(String permission) => hasPermission(permissions, permission);
  bool canAny(Iterable<String> anyOf) => hasAnyPermission(permissions, anyOf);

  Future<void> login({required String email, required String password, required String totpCode}) async {
    final r = await _api.login(email: email.trim(), password: password, totpCode: totpCode.trim());
    _tokens.set(r.tokens);
    _me = r.admin;
    _notice = null;
    notifyListeners();
  }

  /// Changes the signed-in admin's password and keeps this session on the new
  /// token pair (the server revokes all the others).
  Future<void> changePassword({
    required String currentPassword,
    required String newPassword,
    required String totpCode,
  }) async {
    final pair = await _api.changePassword(
      currentPassword: currentPassword,
      newPassword: newPassword,
      totpCode: totpCode.trim(),
    );
    _tokens.set(pair);
  }

  /// Revokes the session on the server (best effort) and forgets the tokens.
  Future<void> logout({String? notice}) async {
    if (_signingOut) return;
    _signingOut = true;
    final rt = _tokens.refreshToken;
    try {
      if (rt != null) await _api.logout(rt);
    } catch (_) {
      // The tokens are dropped anyway; the refresh token expires on its own.
    } finally {
      _tokens.clear();
      _me = null;
      _notice = notice;
      _signingOut = false;
      notifyListeners();
    }
  }

  void _onExpired() {
    if (_signingOut) return;
    _me = null;
    _notice = 'Your session ended. Please sign in again.';
    notifyListeners();
  }

  /// Permissions are copied into the access token, so a role change applies on
  /// refresh; re-read them so the menu matches.
  Future<void> _onRefreshed() async {
    try {
      final me = await _api.me();
      if (_tokens.hasSession) {
        _me = me;
        notifyListeners();
      }
    } catch (_) {
      // Keep the current profile; the next 401/403 is handled where it happens.
    }
  }
}

final authApiProvider = Provider<AuthApi>((ref) => AuthApi(ref.watch(apiClientProvider)));

final sessionProvider = ChangeNotifierProvider<SessionController>(
  (ref) => SessionController(ref.watch(authApiProvider), ref.watch(tokenStoreProvider)),
);

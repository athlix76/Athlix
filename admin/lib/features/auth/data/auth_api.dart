import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/auth/admin_me.dart';
import '../../../core/auth/token_store.dart';

class LoginResult {
  const LoginResult(this.tokens, this.admin);

  final TokenPair tokens;
  final AdminMe admin;
}

/// `POST /admin/auth/login`, `POST /admin/auth/logout`, `GET /admin/me`,
/// `POST /admin/me/password`.
/// (`POST /admin/auth/refresh` lives in [ApiClient].)
class AuthApi {
  AuthApi(this._client);

  final ApiClient _client;

  Future<LoginResult> login({required String email, required String password, required String totpCode}) async {
    final data = asJson(await _client.post('/admin/auth/login', body: {
      'email': email,
      'password': password,
      'totpCode': totpCode,
    }));
    return LoginResult(TokenPair.fromJson(asJson(data['tokens'])), AdminMe.fromJson(asJson(data['admin'])));
  }

  Future<void> logout(String refreshToken) async {
    await _client.post('/admin/auth/logout', body: {'refreshToken': refreshToken});
  }

  /// `POST /admin/me/password`. Every other session is revoked; the response
  /// carries a fresh token pair for this one.
  Future<TokenPair> changePassword({
    required String currentPassword,
    required String newPassword,
    required String totpCode,
  }) async {
    final data = asJson(await _client.post('/admin/me/password', body: {
      'currentPassword': currentPassword,
      'newPassword': newPassword,
      'totpCode': totpCode,
    }));
    return TokenPair.fromJson(asJson(data['tokens']));
  }

  Future<AdminMe> me() async => AdminMe.fromJson(asJson(await _client.get('/admin/me')));
}

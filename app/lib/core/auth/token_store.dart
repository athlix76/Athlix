import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'token_pair.dart';

/// Where the session tokens live. The only production implementation is
/// [SecureTokenStore] (Keychain / Android Keystore-backed storage).
/// Tokens are never written to shared preferences, files, logs or analytics.
abstract class TokenStore {
  Future<TokenPair?> read();
  Future<void> save(TokenPair tokens);
  Future<void> clear();
}

class SecureTokenStore implements TokenStore {
  SecureTokenStore([FlutterSecureStorage? storage])
      : _storage = storage ??
            const FlutterSecureStorage(
              aOptions: AndroidOptions(encryptedSharedPreferences: true),
              iOptions: IOSOptions(accessibility: KeychainAccessibility.first_unlock),
            );

  static const _key = 'athlix.session.tokens';

  final FlutterSecureStorage _storage;

  // In-memory copy so every request does not hit the keychain.
  TokenPair? _cache;
  bool _loaded = false;

  @override
  Future<TokenPair?> read() async {
    if (_loaded) return _cache;
    try {
      final raw = await _storage.read(key: _key);
      if (raw != null && raw.isNotEmpty) {
        final decoded = jsonDecode(raw);
        if (decoded is Map) {
          final pair = TokenPair.fromJson(decoded.cast<String, dynamic>());
          _cache = pair.isValid ? pair : null;
        }
      }
    } catch (_) {
      // Corrupt entry (e.g. restored backup on a new device): start signed out.
      _cache = null;
    }
    _loaded = true;
    return _cache;
  }

  @override
  Future<void> save(TokenPair tokens) async {
    _cache = tokens;
    _loaded = true;
    await _storage.write(key: _key, value: jsonEncode(tokens.toJson()));
  }

  @override
  Future<void> clear() async {
    _cache = null;
    _loaded = true;
    await _storage.delete(key: _key);
  }
}

/// For tests only.
class MemoryTokenStore implements TokenStore {
  MemoryTokenStore([this._tokens]);

  TokenPair? _tokens;

  @override
  Future<TokenPair?> read() async => _tokens;

  @override
  Future<void> save(TokenPair tokens) async {
    _tokens = tokens;
  }

  @override
  Future<void> clear() async {
    _tokens = null;
  }
}

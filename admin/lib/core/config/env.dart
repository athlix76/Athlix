/// Build-time settings, passed with `--dart-define`.
///
/// Only transport and session-safety values live here. Business numbers (fees,
/// limits, plan prices) always come from the backend.
abstract final class Env {
  /// Base URL of the API including `/v1`.
  static const String apiBaseUrl = String.fromEnvironment(
    'API_BASE_URL',
    defaultValue: 'http://localhost:8080/v1',
  );

  /// Minutes without mouse or keyboard activity before the panel signs out.
  static const int idleTimeoutMinutes = int.fromEnvironment(
    'IDLE_TIMEOUT_MINUTES',
    defaultValue: 15,
  );

  /// The access token is refreshed this many seconds before it expires.
  static const int refreshLeewaySeconds = 30;
}

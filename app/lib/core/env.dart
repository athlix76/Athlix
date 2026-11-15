/// Build-time settings, passed with `--dart-define`.
///
/// Nothing secret lives here: only the API base URL and the environment name.
/// Business numbers (fees, limits, versions) come from `GET /bootstrap`.
class Env {
  Env._();

  /// Base URL of the API including the `/v1` prefix.
  /// The default reaches a backend on the host machine from the Android emulator.
  static const String apiBaseUrl = String.fromEnvironment(
    'API_BASE_URL',
    defaultValue: 'http://10.0.2.2:8080/v1',
  );

  /// `local`, `dev`, `staging` or `prod`.
  static const String env = String.fromEnvironment('ENV', defaultValue: 'local');

  static bool get isProd => env == 'prod';

  /// Mock flows (mock DigiLocker, mock listing payment, mock premium) are only
  /// offered outside production. The backend refuses them in prod as well.
  static bool get showMockFlows => !isProd;

  /// Deep link scheme registered on Android and iOS (see app/README.md).
  static const String deepLinkScheme = 'athlix';
}

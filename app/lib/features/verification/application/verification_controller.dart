import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../core/api/api_exception.dart';
import '../../../core/session/session_controller.dart';
import '../data/verification_api.dart';

final verificationStatusProvider = FutureProvider.autoDispose<VerificationStatus>(
  (ref) => ref.watch(verificationApiProvider).status(),
);

/// DigiLocker: the server does the OAuth exchange. The app only opens
/// `authorizationUrl` in a browser tab and later receives
/// `athlix://verification/result?status=...` as a deep link.
class VerificationController {
  VerificationController(this._ref);

  final Ref _ref;

  Future<void> startDigilocker() async {
    final start = await _ref.read(verificationApiProvider).start();
    final uri = Uri.tryParse(start.authorizationUrl);
    if (uri == null || !(uri.isScheme('https') || uri.isScheme('http'))) {
      throw const ApiException(code: ApiErrorCode.upstreamError, message: 'Could not open DigiLocker.');
    }
    final ok = await launchUrl(uri, mode: LaunchMode.inAppBrowserView);
    if (!ok) {
      throw const ApiException(code: ApiErrorCode.upstreamError, message: 'Could not open DigiLocker.');
    }
  }

  /// Called when the deep link arrives: close the tab and re-sync `Me`.
  Future<void> onResult() async {
    try {
      await closeInAppWebView();
    } catch (_) {
      // Not supported on every platform / browser mode.
    }
    _ref.invalidate(verificationStatusProvider);
    await _ref.read(sessionProvider.notifier).refreshMe();
  }

  Future<void> mockComplete(String outcome, {int? nativeDistrictId}) async {
    await _ref.read(verificationApiProvider).mockComplete(outcome, nativeDistrictId: nativeDistrictId);
    _ref.invalidate(verificationStatusProvider);
    await _ref.read(sessionProvider.notifier).refreshMe();
  }
}

final verificationControllerProvider =
    Provider<VerificationController>(VerificationController.new);

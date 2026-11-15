import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/session/session_controller.dart';
import '../data/auth_api.dart';

class AuthFlowState {
  const AuthFlowState({this.mobile, this.challenge, this.sentAt});

  /// Ten digits, kept in memory only for "resend". Never logged or stored.
  final String? mobile;
  final OtpChallenge? challenge;
  final DateTime? sentAt;

  String get maskedMobile => mobile == null ? '' : maskMobile(mobile!);

  /// Seconds until resend is allowed, from `resendAfterSeconds`.
  int resendInSeconds(DateTime now) {
    final c = challenge;
    final at = sentAt;
    if (c == null || at == null) return 0;
    final left = c.resendAfterSeconds - now.difference(at).inSeconds;
    return left < 0 ? 0 : left;
  }

  @override
  String toString() => 'AuthFlowState(<redacted>)';
}

/// Phone → OTP flow. On success the session takes over (tokens to secure storage).
class AuthFlowController extends Notifier<AuthFlowState> {
  @override
  AuthFlowState build() => const AuthFlowState();

  Future<void> requestOtp(String tenDigits) async {
    final challenge = await ref.read(authApiProvider).requestOtp('+91$tenDigits');
    state = AuthFlowState(mobile: tenDigits, challenge: challenge, sentAt: DateTime.now());
  }

  Future<void> resend() async {
    final mobile = state.mobile;
    if (mobile != null) await requestOtp(mobile);
  }

  /// Returns true for a new account.
  Future<bool> verify(String otp) async {
    final challenge = state.challenge;
    if (challenge == null) throw StateError('no OTP requested');
    final result = await ref.read(authApiProvider).verifyOtp(challenge.requestId, otp);
    await ref.read(sessionProvider.notifier).signIn(result.tokens, result.user);
    state = const AuthFlowState();
    return result.isNewUser;
  }
}

final authFlowProvider = NotifierProvider<AuthFlowController, AuthFlowState>(AuthFlowController.new);

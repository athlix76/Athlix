import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/session/session_controller.dart';
import '../data/guardian_api.dart';

class GuardianFlowState {
  const GuardianFlowState({this.request, this.challenge, this.sentAt});

  /// Kept in memory for "resend" only.
  final GuardianRequest? request;
  final GuardianChallenge? challenge;
  final DateTime? sentAt;

  int resendInSeconds(DateTime now) {
    final c = challenge;
    final at = sentAt;
    if (c == null || at == null) return 0;
    final left = c.resendAfterSeconds - now.difference(at).inSeconds;
    return left < 0 ? 0 : left;
  }

  @override
  String toString() => 'GuardianFlowState(<redacted>)';
}

/// Parent consent for minors: the OTP goes to the guardian's mobile.
class GuardianFlowController extends Notifier<GuardianFlowState> {
  @override
  GuardianFlowState build() => const GuardianFlowState();

  Future<void> request(GuardianRequest req) async {
    final challenge = await ref.read(guardianApiProvider).request(req);
    state = GuardianFlowState(request: req, challenge: challenge, sentAt: DateTime.now());
  }

  Future<void> resend() async {
    final req = state.request;
    if (req != null) await request(req);
  }

  Future<void> verify(String otp) async {
    final c = state.challenge;
    if (c == null) throw StateError('no consent requested');
    final me = await ref.read(guardianApiProvider).verify(c.consentId, otp);
    state = const GuardianFlowState();
    ref.read(sessionProvider.notifier).setMe(me);
  }
}

final guardianFlowProvider =
    NotifierProvider<GuardianFlowController, GuardianFlowState>(GuardianFlowController.new);

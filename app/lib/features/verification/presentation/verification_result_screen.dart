import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/router/redirect.dart';
import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/busy_button.dart';
import '../application/verification_controller.dart';
import '../data/verification_api.dart';

/// Target of the deep link `athlix://verification/result?status=verified|minor|failed&reason=`.
/// The query is only a hint for the message; the real state comes from `GET /me`.
class VerificationResultScreen extends ConsumerStatefulWidget {
  const VerificationResultScreen({super.key, this.status, this.reason});

  final String? status;
  final String? reason;

  @override
  ConsumerState<VerificationResultScreen> createState() => _VerificationResultScreenState();
}

class _VerificationResultScreenState extends ConsumerState<VerificationResultScreen> {
  @override
  void initState() {
    super.initState();
    Future.microtask(() => ref.read(verificationControllerProvider).onResult());
  }

  @override
  Widget build(BuildContext context) {
    final minorAge = ref.watch(appConfigProvider).minorAgeYears;
    final (IconData icon, String title, String body) = switch (widget.status) {
      'verified' => (Icons.verified, 'You are verified', 'Thanks! Let us finish your profile.'),
      'minor' => (
          Icons.family_restroom,
          'Verified',
          '${minorAge == null ? 'As a minor' : 'As you are under $minorAge'}, '
              'we need a parent or guardian to give consent next.'
        ),
      _ => (Icons.error_outline, 'Verification not completed', verificationFailureText(widget.reason)),
    };
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(icon, size: 56),
                const SizedBox(height: 16),
                Text(title, style: Theme.of(context).textTheme.headlineSmall),
                const SizedBox(height: 8),
                Text(body, textAlign: TextAlign.center),
                const SizedBox(height: 24),
                BusyButton(
                  label: 'Continue',
                  onPressed: () async {
                    final me = await ref.read(sessionProvider.notifier).refreshMe();
                    if (!context.mounted) return;
                    context.go(me == null ? Routes.splash : routeForStep(me.onboardingStep));
                  },
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

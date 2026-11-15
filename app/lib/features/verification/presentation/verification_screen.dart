import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/env.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/sport_picker.dart';
import '../../location_identity/presentation/geo_pickers.dart';
import '../application/verification_controller.dart';
import '../data/verification_api.dart';

/// Onboarding step `verification` (DigiLocker).
class VerificationScreen extends ConsumerWidget {
  const VerificationScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final status = ref.watch(verificationStatusProvider);
    final controller = ref.read(verificationControllerProvider);
    return Scaffold(
      appBar: AppBar(
        title: const Text('Verify your identity'),
        actions: [
          TextButton(
            onPressed: () => ref.read(sessionProvider.notifier).logout(),
            child: const Text('Sign out'),
          ),
        ],
      ),
      body: AsyncValueView<VerificationStatus>(
        value: status,
        onRetry: () => ref.invalidate(verificationStatusProvider),
        data: (s) => RefreshIndicator(
          onRefresh: () async {
            ref.invalidate(verificationStatusProvider);
            await ref.read(sessionProvider.notifier).refreshMe();
          },
          child: ListView(
            padding: const EdgeInsets.all(16),
            children: [
              const Text(
                'Athlix verifies every member with DigiLocker so players, coaches and organisers '
                'are real people. We keep only your verification status, the last 4 digits of '
                'your Aadhaar, and your name and date of birth (encrypted). We never store your '
                'full Aadhaar number or documents.',
              ),
              const SizedBox(height: 16),
              if (s.status == 'failed')
                Card(
                  child: ListTile(
                    leading: const Icon(Icons.error_outline),
                    title: Text(verificationFailureText(s.failureReason)),
                  ),
                ),
              if (s.status == 'pending')
                const Card(
                  child: ListTile(
                    leading: Icon(Icons.hourglass_top),
                    title: Text('Waiting for DigiLocker. Pull down to refresh.'),
                  ),
                ),
              Text('Attempts left today: ${s.attemptsLeftToday}'),
              const SizedBox(height: 16),
              BusyButton(
                label: 'Verify with DigiLocker',
                icon: Icons.verified_user,
                onPressed: s.attemptsLeftToday <= 0 ? null : controller.startDigilocker,
              ),
              if (Env.showMockFlows) ...[
                const SizedBox(height: 32),
                _MockPanel(controller: controller),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

/// Non-production test outcomes, with an optional native district so the
/// mock creates a verified native identity like DigiLocker would.
class _MockPanel extends StatefulWidget {
  const _MockPanel({required this.controller});

  final VerificationController controller;

  @override
  State<_MockPanel> createState() => _MockPanelState();
}

class _MockPanelState extends State<_MockPanel> {
  int? _stateId;
  int? _districtId;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Test outcomes (${Env.env} only)', style: Theme.of(context).textTheme.labelLarge),
        const SizedBox(height: 8),
        StateDropdown(
          value: _stateId,
          onChanged: (v) => setState(() {
            _stateId = v;
            _districtId = null;
          }),
        ),
        const SizedBox(height: 8),
        DistrictDropdown(
          stateId: _stateId,
          value: _districtId,
          onChanged: (v) => setState(() => _districtId = v),
        ),
        const SizedBox(height: 4),
        Text('Native district (optional)', style: Theme.of(context).textTheme.bodySmall),
        const SizedBox(height: 8),
        Wrap(
          spacing: 8,
          children: [
            for (final o in mockVerificationOutcomes)
              BusyButton(
                label: EnumDropdown.pretty(o),
                style: BusyButtonStyle.outlined,
                onPressed: () => widget.controller.mockComplete(o, nativeDistrictId: _districtId),
              ),
          ],
        ),
      ],
    );
  }
}

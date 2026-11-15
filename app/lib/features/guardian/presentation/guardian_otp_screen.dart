import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/router/routes.dart';
import '../../../core/widgets/busy_button.dart';
import '../application/guardian_flow_controller.dart';

class GuardianOtpScreen extends ConsumerStatefulWidget {
  const GuardianOtpScreen({super.key});

  @override
  ConsumerState<GuardianOtpScreen> createState() => _GuardianOtpScreenState();
}

class _GuardianOtpScreenState extends ConsumerState<GuardianOtpScreen> {
  final _ctrl = TextEditingController();
  Timer? _ticker;

  @override
  void initState() {
    super.initState();
    _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted) setState(() {});
    });
  }

  @override
  void dispose() {
    _ticker?.cancel();
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final flow = ref.watch(guardianFlowProvider);
    final c = flow.challenge;
    if (c == null) {
      return Scaffold(
        appBar: AppBar(),
        body: Center(
          child: FilledButton(
            onPressed: () => context.go(Routes.guardian),
            child: const Text('Enter guardian details'),
          ),
        ),
      );
    }
    final wait = flow.resendInSeconds(DateTime.now());
    return Scaffold(
      appBar: AppBar(title: const Text('Guardian code')),
      body: ListView(
        padding: const EdgeInsets.all(24),
        children: [
          Text('Ask your guardian for the code sent to ${c.guardianMobileMasked}'),
          const SizedBox(height: 16),
          TextField(
            controller: _ctrl,
            keyboardType: TextInputType.number,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: const InputDecoration(labelText: 'Code'),
          ),
          const SizedBox(height: 8),
          BusyButton(
            label: 'Confirm consent',
            onPressed: () async {
              final otp = _ctrl.text.trim();
              if (otp.isEmpty) return;
              await ref.read(guardianFlowProvider.notifier).verify(otp);
            },
          ),
          BusyButton(
            label: wait > 0 ? 'Resend in ${wait}s' : 'Resend code',
            style: BusyButtonStyle.text,
            onPressed: wait > 0 ? null : () => ref.read(guardianFlowProvider.notifier).resend(),
          ),
          TextButton(
            onPressed: () => context.go(Routes.guardian),
            child: const Text('Change guardian details'),
          ),
        ],
      ),
    );
  }
}

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/busy_button.dart';
import '../application/auth_flow_controller.dart';

class OtpScreen extends ConsumerStatefulWidget {
  const OtpScreen({super.key});

  @override
  ConsumerState<OtpScreen> createState() => _OtpScreenState();
}

class _OtpScreenState extends ConsumerState<OtpScreen> {
  final _ctrl = TextEditingController();
  Timer? _ticker;

  @override
  void initState() {
    super.initState();
    // Re-render once a second for the resend countdown.
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
    final flow = ref.watch(authFlowProvider);
    final otpLength = ref.watch(appConfigProvider).otpLength;
    if (flow.challenge == null) {
      return Scaffold(
        appBar: AppBar(),
        body: Center(
          child: FilledButton(
            onPressed: () => context.go(Routes.authPhone),
            child: const Text('Enter your mobile number'),
          ),
        ),
      );
    }
    final wait = flow.resendInSeconds(DateTime.now());
    return Scaffold(
      appBar: AppBar(title: const Text('Verify OTP')),
      body: ListView(
        padding: const EdgeInsets.all(24),
        children: [
          Text('Enter the code sent to ${flow.maskedMobile}'),
          const SizedBox(height: 16),
          TextField(
            controller: _ctrl,
            autofocus: true,
            keyboardType: TextInputType.number,
            autofillHints: const [AutofillHints.oneTimeCode],
            maxLength: otpLength,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: const InputDecoration(labelText: 'OTP'),
          ),
          const SizedBox(height: 8),
          BusyButton(
            label: 'Verify',
            onPressed: () async {
              final otp = _ctrl.text.trim();
              if (otp.isEmpty || (otpLength != null && otp.length != otpLength)) return;
              await ref.read(authFlowProvider.notifier).verify(otp);
              // The router redirect moves on once the session is signed in.
            },
          ),
          const SizedBox(height: 8),
          BusyButton(
            label: wait > 0 ? 'Resend in ${wait}s' : 'Resend OTP',
            style: BusyButtonStyle.text,
            onPressed: wait > 0 ? null : () => ref.read(authFlowProvider.notifier).resend(),
          ),
          TextButton(
            onPressed: () => context.go(Routes.authPhone),
            child: const Text('Change number'),
          ),
        ],
      ),
    );
  }
}

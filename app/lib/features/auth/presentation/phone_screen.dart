import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/models/policy.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/busy_button.dart';
import '../application/auth_flow_controller.dart';
import '../data/auth_api.dart';

class PhoneScreen extends ConsumerStatefulWidget {
  const PhoneScreen({super.key});

  @override
  ConsumerState<PhoneScreen> createState() => _PhoneScreenState();
}

class _PhoneScreenState extends ConsumerState<PhoneScreen> {
  final _ctrl = TextEditingController();
  String? _error;

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final digits = normaliseMobileInput(_ctrl.text);
    if (digits == null) {
      setState(() => _error = 'Enter a 10-digit mobile number');
      return;
    }
    setState(() => _error = null);
    await ref.read(authFlowProvider.notifier).requestOtp(digits);
    if (mounted) context.push(Routes.authOtp);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            const SizedBox(height: 48),
            Text('Athlix', style: Theme.of(context).textTheme.displaySmall),
            const SizedBox(height: 8),
            const Text('Sign in with your mobile number'),
            const SizedBox(height: 24),
            TextField(
              controller: _ctrl,
              keyboardType: TextInputType.phone,
              autofillHints: const [AutofillHints.telephoneNumber],
              inputFormatters: [FilteringTextInputFormatter.allow(RegExp(r'[0-9+\s]'))],
              decoration: InputDecoration(
                labelText: 'Mobile number',
                prefixText: '+91 ',
                errorText: _error,
              ),
              onSubmitted: (_) => _submit(),
            ),
            const SizedBox(height: 16),
            BusyButton(label: 'Send OTP', onPressed: _submit),
            const SizedBox(height: 24),
            Wrap(
              alignment: WrapAlignment.center,
              children: [
                const Text('By continuing you agree to the '),
                InkWell(
                  onTap: () => context.push(Routes.policy(PolicyDocType.terms)),
                  child: Text('Terms', style: TextStyle(color: Theme.of(context).colorScheme.primary)),
                ),
                const Text(' and '),
                InkWell(
                  onTap: () => context.push(Routes.policy(PolicyDocType.privacy)),
                  child: Text('Privacy Policy',
                      style: TextStyle(color: Theme.of(context).colorScheme.primary)),
                ),
                const Text('.'),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

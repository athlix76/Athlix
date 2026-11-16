import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/models/policy.dart';
import '../../../core/router/routes.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/widgets/busy_button.dart';
import '../application/auth_flow_controller.dart';
import '../data/auth_api.dart';
import 'auth_layout.dart';

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
    final linkStyle = TextStyle(color: Theme.of(context).colorScheme.secondary, fontWeight: FontWeight.w800);
    return AuthLayout(
      title: 'Welcome to Athlix.',
      subtitle: 'Sign in with your mobile number to continue your sports journey.',
      children: [
        const FieldLabel('Mobile number'),
        TextField(
          controller: _ctrl,
          keyboardType: TextInputType.phone,
          autofillHints: const [AutofillHints.telephoneNumber],
          inputFormatters: [FilteringTextInputFormatter.allow(RegExp(r'[0-9+\s]'))],
          decoration: InputDecoration(
            hintText: '10-digit mobile number',
            prefixText: '+91 ',
            errorText: _error,
          ),
          onSubmitted: (_) => _submit(),
        ),
        const SizedBox(height: 20),
        BusyButton(label: 'Send OTP', onPressed: _submit),
        const SizedBox(height: 28),
        const Divider(),
        const SizedBox(height: 16),
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Icon(Icons.shield_outlined, size: 18, color: AppTheme.slate),
            const SizedBox(width: 10),
            Expanded(
              child: Wrap(
                children: [
                  const Text('By continuing you agree to the ', style: TextStyle(color: AppTheme.slate)),
                  InkWell(
                    onTap: () => context.push(Routes.policy(PolicyDocType.terms)),
                    child: Text('Terms', style: linkStyle),
                  ),
                  const Text(' and ', style: TextStyle(color: AppTheme.slate)),
                  InkWell(
                    onTap: () => context.push(Routes.policy(PolicyDocType.privacy)),
                    child: Text('Privacy Policy', style: linkStyle),
                  ),
                  const Text('.', style: TextStyle(color: AppTheme.slate)),
                ],
              ),
            ),
          ],
        ),
      ],
    );
  }
}

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/models/policy.dart';
import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/sport_picker.dart';
import '../../auth/data/auth_api.dart';
import '../application/guardian_flow_controller.dart';
import '../data/guardian_api.dart';

/// Onboarding step `guardian` (minors only).
class GuardianScreen extends ConsumerStatefulWidget {
  const GuardianScreen({super.key});

  @override
  ConsumerState<GuardianScreen> createState() => _GuardianScreenState();
}

class _GuardianScreenState extends ConsumerState<GuardianScreen> {
  final _form = GlobalKey<FormState>();
  final _name = TextEditingController();
  final _mobile = TextEditingController();
  String _relation = guardianRelations.first;
  bool _agreed = false;

  @override
  void dispose() {
    _name.dispose();
    _mobile.dispose();
    super.dispose();
  }

  Future<void> _submit(PolicyRef consent) async {
    if (!(_form.currentState?.validate() ?? false)) return;
    final digits = normaliseMobileInput(_mobile.text);
    if (digits == null) return;
    await ref.read(guardianFlowProvider.notifier).request(GuardianRequest(
          guardianName: _name.text.trim(),
          guardianMobile: '+91$digits',
          relation: _relation,
          policyDocumentId: consent.id,
        ));
    if (mounted) context.push(Routes.guardianOtp);
  }

  @override
  Widget build(BuildContext context) {
    final consent = ref.watch(bootstrapProvider).valueOrNull?.policy(PolicyDocType.guardianConsent);
    return Scaffold(
      appBar: AppBar(
        title: const Text('Parent or guardian consent'),
        actions: [
          TextButton(
            onPressed: () => ref.read(sessionProvider.notifier).logout(),
            child: const Text('Sign out'),
          ),
        ],
      ),
      body: Form(
        key: _form,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            const Text(
              'Because you are a minor, a parent or guardian must agree before you can use '
              'Athlix. We will send a one-time code to their mobile number.',
            ),
            const SizedBox(height: 16),
            TextFormField(
              controller: _name,
              decoration: const InputDecoration(labelText: 'Guardian name'),
              validator: (v) => (v == null || v.trim().isEmpty) ? 'Required' : null,
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _mobile,
              keyboardType: TextInputType.phone,
              inputFormatters: [FilteringTextInputFormatter.allow(RegExp(r'[0-9+\s]'))],
              decoration: const InputDecoration(
                labelText: 'Guardian mobile number',
                prefixText: '+91 ',
                helperText: 'Must be different from your own number',
              ),
              validator: (v) =>
                  normaliseMobileInput(v ?? '') == null ? 'Enter a 10-digit mobile number' : null,
            ),
            const SizedBox(height: 12),
            EnumDropdown(
              label: 'Relation',
              values: guardianRelations,
              value: _relation,
              onChanged: (v) => setState(() => _relation = v ?? guardianRelations.first),
            ),
            const SizedBox(height: 12),
            if (consent == null)
              const Text('The consent document could not be loaded. Please try again later.')
            else ...[
              ListTile(
                contentPadding: EdgeInsets.zero,
                title: Text(consent.title),
                subtitle: const Text('Your guardian agrees to this document'),
                trailing: const Icon(Icons.chevron_right),
                onTap: () => context.push(Routes.policy(consent.docType)),
              ),
              CheckboxListTile(
                contentPadding: EdgeInsets.zero,
                value: _agreed,
                onChanged: (v) => setState(() => _agreed = v ?? false),
                controlAffinity: ListTileControlAffinity.leading,
                title: const Text('My guardian has read this document'),
              ),
            ],
            const SizedBox(height: 16),
            BusyButton(
              label: 'Send code to guardian',
              onPressed: consent == null || !_agreed ? null : () => _submit(consent),
            ),
          ],
        ),
      ),
    );
  }
}

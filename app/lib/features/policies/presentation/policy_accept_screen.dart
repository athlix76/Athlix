import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/busy_button.dart';
import '../data/policy_api.dart';

/// Blocks the app until every `Me.pendingPolicies` entry is accepted.
class PolicyAcceptScreen extends ConsumerStatefulWidget {
  const PolicyAcceptScreen({super.key});

  @override
  ConsumerState<PolicyAcceptScreen> createState() => _PolicyAcceptScreenState();
}

class _PolicyAcceptScreenState extends ConsumerState<PolicyAcceptScreen> {
  bool _agreed = false;

  @override
  Widget build(BuildContext context) {
    final pending = ref.watch(meProvider)?.pendingPolicies ?? const [];
    return Scaffold(
      appBar: AppBar(title: const Text('Before you continue')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          const Text('Please read and accept these documents to use Athlix.'),
          const SizedBox(height: 12),
          for (final p in pending)
            Card(
              child: ListTile(
                title: Text(p.title),
                subtitle: Text('Version ${p.version}'),
                trailing: const Icon(Icons.chevron_right),
                onTap: () => context.push(Routes.policy(p.docType)),
              ),
            ),
          CheckboxListTile(
            value: _agreed,
            onChanged: (v) => setState(() => _agreed = v ?? false),
            controlAffinity: ListTileControlAffinity.leading,
            title: const Text('I have read and accept these documents'),
          ),
          const SizedBox(height: 8),
          BusyButton(
            label: 'Accept and continue',
            onPressed: !_agreed || pending.isEmpty
                ? null
                : () async {
                    final me = await ref
                        .read(policyApiProvider)
                        .accept([for (final p in pending) p.id]);
                    ref.read(sessionProvider.notifier).setMe(me);
                  },
          ),
          const SizedBox(height: 8),
          TextButton(
            onPressed: () => ref.read(sessionProvider.notifier).logout(),
            child: const Text('Sign out'),
          ),
        ],
      ),
    );
  }
}

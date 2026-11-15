import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/busy_button.dart';

/// 403 `ACCOUNT_SUSPENDED` or `Me.status = suspended`.
class SuspendedScreen extends ConsumerWidget {
  const SuspendedScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final support = ref.watch(appConfigProvider).supportEmail;
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(Icons.block, size: 48),
                const SizedBox(height: 16),
                Text('Account suspended', style: Theme.of(context).textTheme.headlineSmall),
                const SizedBox(height: 8),
                Text(
                  support == null
                      ? 'Your account has been suspended.'
                      : 'Your account has been suspended. Contact $support if you think this is a mistake.',
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 16),
                BusyButton(
                  label: 'Sign out',
                  style: BusyButtonStyle.outlined,
                  onPressed: () => ref.read(sessionProvider.notifier).logout(),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

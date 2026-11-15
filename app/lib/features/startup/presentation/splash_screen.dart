import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/error_view.dart';

/// Shown while bootstrap and the session load. The router leaves this screen
/// on its own once both are known.
class SplashScreen extends ConsumerWidget {
  const SplashScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final boot = ref.watch(bootstrapProvider);
    final session = ref.watch(sessionProvider);
    final Object? error = boot.hasError ? boot.error : session.loadError;

    return Scaffold(
      body: SafeArea(
        child: Center(
          child: error != null
              ? ErrorView(
                  error: error,
                  onRetry: () {
                    if (boot.hasError) ref.read(bootstrapProvider.notifier).reload();
                    if (session.loadError != null) ref.read(sessionProvider.notifier).restore();
                  },
                )
              : Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text('Athlix', style: Theme.of(context).textTheme.displaySmall),
                    const SizedBox(height: 24),
                    const CircularProgressIndicator(),
                  ],
                ),
        ),
      ),
    );
  }
}

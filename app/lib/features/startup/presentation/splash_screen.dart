import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/athlix_logo.dart';

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
      // Navy like the native splash; the error view needs the light body.
      backgroundColor: error != null ? null : AppTheme.navy,
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
              : const Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    AthlixWordmark(height: 150),
                    SizedBox(height: 28),
                    SizedBox(width: 28, height: 28, child: CircularProgressIndicator(strokeWidth: 2.5)),
                  ],
                ),
        ),
      ),
    );
  }
}

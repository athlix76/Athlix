import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'core/router/app_router.dart';
import 'core/session/session_controller.dart';
import 'core/theme/app_theme.dart';
import 'features/notifications/application/push_service.dart';

/// Root widget: theme, router, and side effects tied to the session
/// (push registration starts after sign-in; logout stops it in settings).
class AthlixApp extends ConsumerWidget {
  const AthlixApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.listen<SessionState>(sessionProvider, (prev, next) {
      final was = prev?.isSignedIn ?? false;
      if (next.isSignedIn && !was) {
        ref.read(pushServiceProvider).start();
      }
    });
    final router = ref.watch(routerProvider);
    return MaterialApp.router(
      title: 'Athlix',
      debugShowCheckedModeBanner: false,
      // One dark look everywhere, whatever the phone's light/dark setting.
      theme: AppTheme.dark(),
      darkTheme: AppTheme.dark(),
      themeMode: ThemeMode.dark,
      routerConfig: router,
    );
  }
}

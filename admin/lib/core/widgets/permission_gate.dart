import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../features/auth/application/session_controller.dart';
import 'api_error_view.dart';

/// Shows [child] only when the admin has one of [anyOf]; else the 403 view.
class PermissionGate extends ConsumerWidget {
  const PermissionGate({super.key, required this.anyOf, required this.child});

  final List<String> anyOf;
  final Widget child;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final session = ref.watch(sessionProvider);
    if (session.canAny(anyOf)) return child;
    return ForbiddenView(permission: anyOf.join(' or '));
  }
}

/// Renders [child] only when the admin has [permission] (for buttons).
class IfCan extends ConsumerWidget {
  const IfCan(this.permission, {super.key, required this.child});

  final String permission;
  final Widget child;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return ref.watch(sessionProvider).can(permission) ? child : const SizedBox.shrink();
  }
}

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../application/follow_controller.dart';

/// Asks, then blocks [userId]. Returns true when blocked.
Future<bool> confirmBlock(BuildContext context, WidgetRef ref, {required int userId, required String name}) async {
  final ok = await confirm(
    context,
    title: 'Block $name?',
    body: 'They will not be able to see your profile or message you, and you will stop following each other.',
    ok: 'Block',
  );
  if (!ok) return false;
  try {
    await ref.read(followControllerProvider).block(userId);
    if (context.mounted) showInfoSnack(context, '$name is blocked.');
    return true;
  } catch (e) {
    if (context.mounted) showErrorSnack(context, e);
    return false;
  }
}

/// Asks, then unblocks [userId]. Returns true when unblocked.
Future<bool> confirmUnblock(BuildContext context, WidgetRef ref, {required int userId, required String name}) async {
  final ok = await confirm(context, title: 'Unblock $name?', ok: 'Unblock');
  if (!ok) return false;
  try {
    await ref.read(followControllerProvider).unblock(userId);
    if (context.mounted) showInfoSnack(context, '$name is unblocked.');
    return true;
  } catch (e) {
    if (context.mounted) showErrorSnack(context, e);
    return false;
  }
}

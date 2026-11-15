import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../application/eligibility_view.dart';

/// Bottom sheet for a 403 `NOT_ELIGIBLE` (register, contact). Returns true
/// when [error] was handled, so callers can fall back to a snack otherwise.
Future<bool> showNotEligibleSheetIfNeeded(BuildContext context, Object error) async {
  final info = NotEligibleInfo.fromError(error);
  if (info == null || !context.mounted) return false;
  await showNotEligibleSheet(context, info);
  return true;
}

Future<void> showNotEligibleSheet(BuildContext context, NotEligibleInfo info) {
  return showModalBottomSheet<void>(
    context: context,
    showDragHandle: true,
    builder: (ctx) => SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text('Not eligible', style: Theme.of(ctx).textTheme.titleLarge),
            const SizedBox(height: 12),
            Text(info.text),
            const SizedBox(height: 20),
            if (info.actionLabel != null && info.actionRoute != null)
              FilledButton(
                onPressed: () {
                  Navigator.of(ctx).pop();
                  context.push(info.actionRoute!);
                },
                child: Text(info.actionLabel!),
              ),
            TextButton(onPressed: () => Navigator.of(ctx).pop(), child: const Text('Close')),
          ],
        ),
      ),
    ),
  );
}

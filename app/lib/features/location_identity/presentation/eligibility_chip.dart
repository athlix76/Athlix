import 'package:flutter/material.dart';

import '../application/eligibility_view.dart';

/// Small chip with the server's `myEligibility.tag` ("Native: Thanjavur").
class EligibilityChip extends StatelessWidget {
  const EligibilityChip({super.key, required this.view});

  final EligibilityView view;

  @override
  Widget build(BuildContext context) {
    final tag = view.tag;
    if (tag == null || tag.isEmpty) return const SizedBox.shrink();
    final scheme = Theme.of(context).colorScheme;
    final bg = !view.eligible
        ? scheme.surfaceContainerHighest
        : view.pending
            ? scheme.tertiaryContainer
            : scheme.secondaryContainer;
    return Chip(
      visualDensity: VisualDensity.compact,
      materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
      avatar: Icon(
        view.eligible ? (view.pending ? Icons.hourglass_top : Icons.place) : Icons.block,
        size: 16,
      ),
      label: Text(tag),
      backgroundColor: bg,
      side: BorderSide.none,
    );
  }
}

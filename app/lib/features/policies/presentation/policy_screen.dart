import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/format/dates.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/simple_markdown.dart';
import '../application/policy_providers.dart';

/// Reads one policy document (terms, privacy, refund, consent, ...).
class PolicyScreen extends ConsumerWidget {
  const PolicyScreen({super.key, required this.docType});

  final String docType;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final doc = ref.watch(policyDocumentProvider(docType));
    return Scaffold(
      appBar: AppBar(title: Text(doc.valueOrNull?.title ?? humanize(docType))),
      body: AsyncValueView(
        value: doc,
        onRetry: () => ref.invalidate(policyDocumentProvider(docType)),
        data: (d) => ListView(
          padding: const EdgeInsets.all(16),
          children: [
            Text('Version ${d.version}${d.effectiveFrom == null ? '' : ' · from ${formatDate(d.effectiveFrom)}'}',
                style: Theme.of(context).textTheme.labelMedium),
            const SizedBox(height: 12),
            SimpleMarkdown(d.contentMd),
          ],
        ),
      ),
    );
  }
}

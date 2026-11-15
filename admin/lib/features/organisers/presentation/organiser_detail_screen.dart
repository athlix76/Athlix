import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/format/format.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../application/organisers_providers.dart';
import '../data/organisers_api.dart';

class OrganiserDetailScreen extends ConsumerWidget {
  const OrganiserDetailScreen({super.key, required this.userId});

  final int userId;

  Future<void> _approve(BuildContext context, WidgetRef ref) async {
    final api = ref.read(organisersApiProvider);
    final r = await confirmAction<OrganiserApplication>(
      context,
      title: 'Approve organiser?',
      message: 'The user can create and list events. They are notified.',
      confirmLabel: 'Approve',
      reason: const ReasonSpec(label: 'Note to the organiser', required: false),
      run: (note) => api.approve(userId, note),
      successMessage: 'Organiser approved',
    );
    if (r != null) ref.invalidate(organiserDetailProvider(userId));
  }

  Future<void> _reject(BuildContext context, WidgetRef ref) async {
    final api = ref.read(organisersApiProvider);
    final r = await confirmAction<OrganiserApplication>(
      context,
      title: 'Reject application?',
      message: 'The organiser sees your note and can fix and resubmit.',
      confirmLabel: 'Reject',
      destructive: true,
      reason: const ReasonSpec(label: 'Note to the organiser', hint: 'What needs to change'),
      run: (note) => api.reject(userId, note!),
      successMessage: 'Application rejected',
    );
    if (r != null) ref.invalidate(organiserDetailProvider(userId));
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(organiserDetailProvider(userId));
    return PageScaffold(
      title: 'Organiser application',
      actions: [
        IconButton(tooltip: 'Reload', icon: const Icon(Icons.refresh), onPressed: () => ref.invalidate(organiserDetailProvider(userId))),
      ],
      body: AsyncBody<OrganiserApplication>(
        value: value,
        onRetry: () => ref.invalidate(organiserDetailProvider(userId)),
        data: (a) => SingleChildScrollView(
          child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            SectionCard(
              title: a.orgName ?? '(no organisation name)',
              actions: [
                StatusChip(a.status),
                if (a.status == 'submitted') ...[
                  const SizedBox(width: 8),
                  OutlinedButton(onPressed: () => _reject(context, ref), child: const Text('Reject')),
                  const SizedBox(width: 8),
                  FilledButton(onPressed: () => _approve(context, ref), child: const Text('Approve')),
                ],
              ],
              child: KeyValueList([
                ('Applicant', textValue(a.user.label)),
                ('Verified user', textValue(a.user.isVerified ? 'yes' : 'no')),
                ('Organisation type', textValue(a.orgType)),
                ('Location', textValue([a.user.city, a.user.state].whereType<String>().join(', '))),
                ('Submitted', textValue(formatDateTime(a.submittedAt))),
                ('Reviewed', textValue(formatDateTime(a.reviewedAt))),
                ('Review note', textValue(a.reviewNote)),
              ]),
            ),
            for (final s in a.steps) _StepCard(s),
          ]),
        ),
      ),
    );
  }
}

class _StepCard extends StatelessWidget {
  const _StepCard(this.s);

  final OrganiserStepSubmission s;

  @override
  Widget build(BuildContext context) {
    final Widget value;
    if (!s.submitted) {
      value = Text(s.isRequired ? 'Missing (required)' : 'Not provided');
    } else {
      value = switch (s.inputType) {
        'document' => s.documentUrl == null
            ? const Text('No document')
            : Align(
                alignment: Alignment.centerLeft,
                child: OutlinedButton.icon(
                  onPressed: () => openUrl(s.documentUrl!),
                  icon: const Icon(Icons.open_in_new, size: 18),
                  label: const Text('Open document (link expires)'),
                ),
              ),
        'checkbox' => Text(s.checked == true ? 'Confirmed' : 'Not confirmed'),
        _ => SelectableText(s.valueText ?? '-'),
      };
    }
    return SectionCard(
      title: '${s.title}${s.isRequired ? ' *' : ''}',
      actions: [if (s.status != null) StatusChip(s.status!)],
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        if (s.description.isNotEmpty) Text(s.description, style: Theme.of(context).textTheme.bodySmall),
        const SizedBox(height: 8),
        value,
        if (s.reviewNote != null) Padding(padding: const EdgeInsets.only(top: 8), child: Text('Note: ${s.reviewNote}')),
      ]),
    );
  }
}

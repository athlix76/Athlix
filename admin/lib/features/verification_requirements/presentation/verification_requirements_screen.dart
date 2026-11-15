import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../application/verification_requirements_providers.dart';
import '../data/verification_requirements_api.dart';

/// Which checks each role needs, and what each check unlocks (gates).
class VerificationRequirementsScreen extends ConsumerWidget {
  const VerificationRequirementsScreen({super.key});

  Future<void> _toggle(BuildContext context, WidgetRef ref, VerificationRequirement r, bool to) async {
    final api = ref.read(verificationRequirementsApiProvider);
    final res = await confirmAction<VerificationRequirement>(
      context,
      title: '${to ? 'Require' : 'Stop requiring'} ${r.requirement} for ${r.role}?',
      message: to
          ? 'Users with this role are blocked from ${r.gates.join(', ')} until they complete it.'
          : 'Users with this role can ${r.gates.join(', ')} without it.',
      confirmLabel: to ? 'Require' : 'Stop requiring',
      destructive: !to,
      run: (_) => api.setRequired(r.id, to),
      successMessage: 'Saved',
    );
    if (res != null) ref.invalidate(verificationRequirementsProvider);
  }

  Future<void> _add(BuildContext context, WidgetRef ref) async {
    final api = ref.read(verificationRequirementsApiProvider);
    var role = requirementRoles.first;
    var requirement = requirementKinds.first;
    var isRequired = true;
    final gates = <String>{};
    final ok = await showEditorDialog(
      context,
      title: 'Add requirement',
      builder: (ctx, setState) => formColumn([
        FormDropdown<String>(label: 'Role', value: role, items: [for (final r in requirementRoles) (r, r)], onChanged: (v) => setState(() => role = v ?? role)),
        FormDropdown<String>(
          label: 'Requirement',
          value: requirement,
          items: [for (final r in requirementKinds) (r, r)],
          onChanged: (v) => setState(() => requirement = v ?? requirement),
        ),
        Text('Gates (what it unlocks)', style: Theme.of(ctx).textTheme.labelLarge),
        Wrap(spacing: 8, children: [
          for (final g in requirementGates)
            FilterChip(
              label: Text(g),
              selected: gates.contains(g),
              onSelected: (on) => setState(() => on ? gates.add(g) : gates.remove(g)),
            ),
        ]),
        SwitchListTile(title: const Text('Required'), value: isRequired, onChanged: (v) => setState(() => isRequired = v)),
      ]),
      validate: () => {if (gates.isEmpty) 'gates': 'choose at least one'},
      confirmMessage: 'Add this requirement? It applies to existing and new users straight away.',
      save: () => api.create(role: role, requirement: requirement, isRequired: isRequired, gates: gates.toList()),
    );
    if (ok) ref.invalidate(verificationRequirementsProvider);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(verificationRequirementsProvider);
    final canWrite = ref.watch(sessionProvider).can(Perm.configWrite);
    return PageScaffold(
      title: 'Verification requirements',
      subtitle: 'Turning a requirement on blocks the gated actions until the user completes it.',
      actions: [
        IconButton(tooltip: 'Reload', icon: const Icon(Icons.refresh), onPressed: () => ref.invalidate(verificationRequirementsProvider)),
        if (canWrite) FilledButton.icon(onPressed: () => _add(context, ref), icon: const Icon(Icons.add), label: const Text('Add')),
      ],
      body: AsyncBody<List<VerificationRequirement>>(
        value: value,
        onRetry: () => ref.invalidate(verificationRequirementsProvider),
        data: (rows) => rows.isEmpty
            ? const Center(child: Text('No requirements.'))
            : ListView.separated(
                itemCount: rows.length,
                separatorBuilder: (_, __) => const Divider(height: 1),
                itemBuilder: (context, i) {
                  final r = rows[i];
                  return ListTile(
                    title: Text('${r.role}  ·  ${r.requirement}'),
                    subtitle: Text('Gates: ${r.gates.isEmpty ? '-' : r.gates.join(', ')}'),
                    trailing: Switch(value: r.isRequired, onChanged: canWrite ? (v) => _toggle(context, ref, r, v) : null),
                  );
                },
              ),
      ),
    );
  }
}

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../application/organiser_steps_providers.dart';
import '../data/organiser_steps_api.dart';

final _codeRe = RegExp(r'^[a-z][a-z0-9_]{1,39}$');

/// The organiser verification checklist. Steps are deactivated, not deleted,
/// so earlier submissions keep their link.
class OrganiserStepsScreen extends ConsumerWidget {
  const OrganiserStepsScreen({super.key});

  Future<void> _edit(BuildContext context, WidgetRef ref, OrganiserStep? s) async {
    final api = ref.read(organiserStepsApiProvider);
    // Not disposed: the dialog's exit animation may still use them (see editor_dialog.dart).
    final code = TextEditingController(text: s?.stepCode ?? '');
    final title = TextEditingController(text: s?.title ?? '');
    final description = TextEditingController(text: s?.description ?? '');
    final order = TextEditingController(text: '${s?.sortOrder ?? 0}');
    var inputType = s?.inputType ?? stepInputTypes.first;
    var isRequired = s?.isRequired ?? true;
    var active = s?.active ?? true;
    final ok = await showEditorDialog(
      context,
      title: s == null ? 'Add step' : 'Edit ${s.title}',
      builder: (ctx, setState) => formColumn([
        FormText(label: 'Step code (lower_snake, cannot change later)', controller: code, enabled: s == null),
        FormText(label: 'Title', controller: title),
        FormText(label: 'Description shown to organisers', controller: description, maxLines: 4),
        FormDropdown<String>(
          label: 'Input type',
          value: inputType,
          items: [for (final t in stepInputTypes) (t, t)],
          onChanged: (v) => setState(() => inputType = v ?? inputType),
        ),
        FormText(label: 'Sort order', controller: order, keyboardType: TextInputType.number),
        SwitchListTile(title: const Text('Required'), value: isRequired, onChanged: (v) => setState(() => isRequired = v)),
        if (s != null)
          SwitchListTile(
            title: const Text('Active'),
            subtitle: const Text('Inactive steps are not asked for; earlier answers are kept.'),
            value: active,
            onChanged: (v) => setState(() => active = v),
          ),
      ]),
      validate: () => {
        if (s == null && !_codeRe.hasMatch(code.text.trim())) 'stepCode': 'lower case letters, digits and _ (2 to 40)',
        if (title.text.trim().isEmpty) 'title': 'is required',
        if (int.tryParse(order.text.trim()) == null) 'sortOrder': 'must be a whole number',
      },
      confirmMessage: s == null
          ? 'Add this step? Organisers who have not submitted yet must complete it.'
          : 'Save changes? Organisers see them on their next visit.',
      save: () {
        final body = <String, Object?>{
          'title': title.text.trim(),
          'description': description.text.trim(),
          'inputType': inputType,
          'isRequired': isRequired,
          'sortOrder': int.parse(order.text.trim()),
        };
        if (s == null) return api.create({'stepCode': code.text.trim(), ...body});
        return api.update(s.id, {...body, 'active': active});
      },
    );
    if (ok) ref.invalidate(organiserStepsProvider);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(organiserStepsProvider);
    final canWrite = ref.watch(sessionProvider).can(Perm.configWrite);
    return PageScaffold(
      title: 'Organiser verification steps',
      actions: [
        IconButton(tooltip: 'Reload', icon: const Icon(Icons.refresh), onPressed: () => ref.invalidate(organiserStepsProvider)),
        if (canWrite) FilledButton.icon(onPressed: () => _edit(context, ref, null), icon: const Icon(Icons.add), label: const Text('Add step')),
      ],
      body: AsyncBody<List<OrganiserStep>>(
        value: value,
        onRetry: () => ref.invalidate(organiserStepsProvider),
        data: (rows) {
          final sorted = [...rows]..sort((a, b) => a.sortOrder.compareTo(b.sortOrder));
          if (sorted.isEmpty) return const Center(child: Text('No steps.'));
          return ListView.separated(
            itemCount: sorted.length,
            separatorBuilder: (_, __) => const Divider(height: 1),
            itemBuilder: (context, i) {
              final s = sorted[i];
              return ListTile(
                leading: Text('${s.sortOrder}'),
                title: Text('${s.title}${s.isRequired ? ' *' : ''}'),
                subtitle: Text('${s.stepCode}  ·  ${s.inputType}${s.description.isEmpty ? '' : '  ·  ${s.description}'}'),
                trailing: Wrap(spacing: 8, crossAxisAlignment: WrapCrossAlignment.center, children: [
                  StatusChip(s.active ? 'active' : 'inactive'),
                  if (canWrite) IconButton(tooltip: 'Edit', icon: const Icon(Icons.edit_outlined), onPressed: () => _edit(context, ref, s)),
                ]),
              );
            },
          );
        },
      ),
    );
  }
}

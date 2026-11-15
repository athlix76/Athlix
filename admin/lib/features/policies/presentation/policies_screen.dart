import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../application/policies_providers.dart';
import '../data/policies_api.dart';

/// Versioned policy documents (markdown). A new version is created inactive;
/// activating it deactivates the previous version of that type. When
/// `requiresAcceptance` is on, the app blocks users until they accept it.
class PoliciesScreen extends ConsumerStatefulWidget {
  const PoliciesScreen({super.key});

  @override
  ConsumerState<PoliciesScreen> createState() => _PoliciesScreenState();
}

class _PoliciesScreenState extends ConsumerState<PoliciesScreen> {
  String? _docType;

  void _reload() => ref.invalidate(policiesProvider);

  Future<void> _newVersion(List<PolicyDocument> all, {String? docType}) async {
    final api = ref.read(policiesApiProvider);
    var type = docType ?? _docType ?? policyDocTypes.first;
    PolicyDocument? latestOf(String t) {
      final of = all.where((p) => p.docType == t).toList()..sort((a, b) => b.version.compareTo(a.version));
      return of.isEmpty ? null : of.first;
    }

    final base = latestOf(type);
    // Not disposed: the dialog's exit animation may still use them (see editor_dialog.dart).
    final title = TextEditingController(text: base?.title ?? '');
    final content = TextEditingController(text: base?.contentMd ?? '');
    var requires = base?.requiresAcceptance ?? true;
    final ok = await showEditorDialog(
      context,
      title: 'New policy version',
      width: 820,
      builder: (ctx, setState) => formColumn([
        FormDropdown<String>(
          label: 'Document type',
          value: type,
          items: [for (final t in policyDocTypes) (t, t)],
          onChanged: (v) => setState(() => type = v ?? type),
        ),
        FormText(label: 'Title', controller: title),
        TextField(
          controller: content,
          minLines: 14,
          maxLines: 28,
          keyboardType: TextInputType.multiline,
          style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
          decoration: const InputDecoration(labelText: 'Content (markdown)', border: OutlineInputBorder(), alignLabelWithHint: true),
        ),
        SwitchListTile(
          title: const Text('Requires acceptance'),
          subtitle: const Text('Once activated, users must accept this version before they continue.'),
          value: requires,
          onChanged: (v) => setState(() => requires = v),
        ),
        Text('Prefilled from the latest version of the chosen type when it was opened. Saving creates an inactive version.',
            style: Theme.of(ctx).textTheme.bodySmall),
      ]),
      validate: () => {
        if (title.text.trim().isEmpty) 'title': 'is required',
        if (content.text.trim().isEmpty) 'contentMd': 'is required',
      },
      confirmMessage: 'Create a new inactive version? Nothing changes for users until you activate it.',
      confirmDetails: () {
        final prev = latestOf(type);
        return prev == null ? null : DiffView(before: prev.contentMd, after: content.text);
      },
      save: () => api.create(docType: type, title: title.text.trim(), contentMd: content.text, requiresAcceptance: requires),
      successMessage: 'Version created (inactive)',
    );
    if (ok) _reload();
  }

  Future<void> _activate(PolicyDocument p) async {
    final api = ref.read(policiesApiProvider);
    final r = await confirmAction<PolicyDocument>(
      context,
      title: 'Activate ${p.docType} v${p.version}?',
      message: p.requiresAcceptance
          ? 'The previous version is deactivated and every user must accept this one before continuing.'
          : 'The previous version is deactivated. Users see this one from now on.',
      confirmLabel: 'Activate',
      run: (_) => api.activate(p.id),
      successMessage: 'Activated',
    );
    if (r != null) _reload();
  }

  @override
  Widget build(BuildContext context) {
    final value = ref.watch(policiesProvider(_docType));
    final canWrite = ref.watch(sessionProvider).can(Perm.policiesWrite);
    return PageScaffold(
      title: 'Policies',
      actions: [
        IconButton(tooltip: 'Reload', icon: const Icon(Icons.refresh), onPressed: _reload),
        if (canWrite)
          FilledButton.icon(
            onPressed: value.hasValue ? () => _newVersion(value.requireValue) : null,
            icon: const Icon(Icons.add),
            label: const Text('New version'),
          ),
      ],
      filters: FilterDropdown(label: 'Type', value: _docType, options: policyDocTypes, onChanged: (v) => setState(() => _docType = v)),
      body: AsyncBody<List<PolicyDocument>>(
        value: value,
        onRetry: _reload,
        data: (items) {
          if (items.isEmpty) return const Center(child: Text('No policy documents.'));
          final sorted = [...items]..sort((a, b) {
              final t = a.docType.compareTo(b.docType);
              return t != 0 ? t : b.version.compareTo(a.version);
            });
          return ListView(children: [
            for (final p in sorted)
              Card(
                child: ExpansionTile(
                  title: Text('${p.docType}  ·  v${p.version}  ·  ${p.title}'),
                  subtitle: Text([
                    p.requiresAcceptance ? 'requires acceptance' : 'no acceptance needed',
                    'created ${formatDateTime(p.createdAt)}',
                    if (p.effectiveFrom != null) 'effective ${formatDateTime(p.effectiveFrom)}',
                  ].join('  ·  ')),
                  trailing: Wrap(spacing: 8, crossAxisAlignment: WrapCrossAlignment.center, children: [
                    StatusChip(p.active ? 'active' : 'inactive'),
                    if (canWrite && !p.active) OutlinedButton(onPressed: () => _activate(p), child: const Text('Activate')),
                  ]),
                  childrenPadding: const EdgeInsets.all(16),
                  children: [
                    Align(
                      alignment: Alignment.centerLeft,
                      child: SelectableText(p.contentMd, style: const TextStyle(fontFamily: 'monospace', fontSize: 13)),
                    ),
                    if (canWrite)
                      Align(
                        alignment: Alignment.centerRight,
                        child: TextButton(
                          onPressed: () => _newVersion(items, docType: p.docType),
                          child: const Text('New version from this type'),
                        ),
                      ),
                  ],
                ),
              ),
          ]);
        },
      ),
    );
  }
}

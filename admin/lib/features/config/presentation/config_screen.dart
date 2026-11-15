import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../application/config_providers.dart';
import '../application/config_value_codec.dart';
import '../data/config_api.dart';

/// Runtime config (`app_config`), grouped by category. Saved values are live
/// on every server within about 15 seconds. Editing sends the version the
/// admin saw, so a concurrent change gives 409 instead of being overwritten.
class ConfigScreen extends ConsumerStatefulWidget {
  const ConfigScreen({super.key});

  @override
  ConsumerState<ConfigScreen> createState() => _ConfigScreenState();
}

class _ConfigScreenState extends ConsumerState<ConfigScreen> {
  final _search = TextEditingController();
  String _q = '';
  String? _category;

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  void _reload() => ref.invalidate(configEntriesProvider);

  @override
  Widget build(BuildContext context) {
    final value = ref.watch(configEntriesProvider);
    final canWrite = ref.watch(sessionProvider).can(Perm.configWrite);
    final categories = {...?value.valueOrNull?.map((e) => e.category)}.toList()..sort();
    return PageScaffold(
      title: 'Config',
      subtitle: 'Business rules and switches. Changes apply within about 15 seconds, no deploy.',
      actions: [IconButton(tooltip: 'Reload', icon: const Icon(Icons.refresh), onPressed: _reload)],
      filters: Wrap(spacing: 12, runSpacing: 12, children: [
        FilterDropdown(label: 'Category', value: _category, options: categories, onChanged: (v) => setState(() => _category = v)),
        FilterTextField(label: 'Key or description', controller: _search, onSubmitted: (v) => setState(() => _q = v.trim().toLowerCase())),
      ]),
      body: AsyncBody<List<ConfigEntry>>(
        value: value,
        onRetry: _reload,
        data: (entries) {
          final shown = entries
              .where((e) => _category == null || e.category == _category)
              .where((e) => _q.isEmpty || e.key.toLowerCase().contains(_q) || e.description.toLowerCase().contains(_q))
              .toList()
            ..sort((a, b) => a.key.compareTo(b.key));
          if (shown.isEmpty) return const Center(child: Text('No config entries match.'));
          final groups = <String, List<ConfigEntry>>{};
          for (final e in shown) {
            groups.putIfAbsent(e.category, () => []).add(e);
          }
          final names = groups.keys.toList()..sort();
          return ListView(children: [
            for (final c in names)
              SectionCard(
                title: c,
                child: Column(children: [
                  for (final e in groups[c]!)
                    _ConfigTile(
                      entry: e,
                      canWrite: canWrite,
                      onEdit: () async {
                        if (await editConfigEntry(context, ref, e)) _reload();
                      },
                    ),
                ]),
              ),
          ]);
        },
      ),
    );
  }
}

class _ConfigTile extends StatelessWidget {
  const _ConfigTile({required this.entry, required this.canWrite, required this.onEdit});

  final ConfigEntry entry;
  final bool canWrite;
  final VoidCallback onEdit;

  @override
  Widget build(BuildContext context) {
    final text = configValueToText(entry.value, entry.valueType);
    final preview = text.length > 160 ? '${text.substring(0, 160)}…' : text;
    final small = Theme.of(context).textTheme.bodySmall;
    return ListTile(
      contentPadding: EdgeInsets.zero,
      title: SelectableText(entry.key, style: const TextStyle(fontFamily: 'monospace')),
      subtitle: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        if (entry.description.isNotEmpty) Text(entry.description),
        const SizedBox(height: 4),
        Text(preview.replaceAll('\n', entry.valueType == ConfigValueType.stringList ? ', ' : ' '),
            style: const TextStyle(fontFamily: 'monospace', fontSize: 12)),
        Text(
          '${entry.valueType}  ·  v${entry.version}  ·  ${entry.isPublic ? 'public (sent to the app)' : 'server only'}'
          '  ·  updated ${formatDateTime(entry.updatedAt)}',
          style: small,
        ),
      ]),
      trailing: Wrap(spacing: 4, children: [
        IconButton(tooltip: 'History', icon: const Icon(Icons.history), onPressed: () => showConfigHistory(context, entry)),
        if (canWrite) IconButton(tooltip: 'Edit', icon: const Icon(Icons.edit_outlined), onPressed: onEdit),
      ]),
    );
  }
}

/// Typed editor: a switch for bool, a number field for int / decimal, one
/// item per line for string_list, monospace JSON for json. Shows a diff
/// before saving. Returns true when saved.
Future<bool> editConfigEntry(BuildContext context, WidgetRef ref, ConfigEntry e) async {
  final api = ref.read(configApiProvider);
  final before = configValueToText(e.value, e.valueType);
  // Not disposed: the dialog's exit animation may still use it (see editor_dialog.dart).
  final text = TextEditingController(text: before);
  var boolValue = e.value == true;
  Object? parsed;

  String currentText() => e.valueType == ConfigValueType.boolean ? '$boolValue' : text.text;

  return showEditorDialog(
    context,
    title: 'Edit ${e.key}',
    width: 640,
    builder: (ctx, setState) {
      final check = parseConfigValue(currentText(), e.valueType);
      final multi = e.valueType == ConfigValueType.stringList || e.valueType == ConfigValueType.json;
      return formColumn([
        if (e.description.isNotEmpty) Text(e.description),
        Text('Type ${e.valueType}  ·  version ${e.version}', style: Theme.of(ctx).textTheme.bodySmall),
        if (e.valueType == ConfigValueType.boolean)
          SwitchListTile(
            title: Text(boolValue ? 'true' : 'false'),
            value: boolValue,
            onChanged: (v) => setState(() => boolValue = v),
          )
        else
          TextField(
            controller: text,
            minLines: multi ? 6 : 1,
            maxLines: multi ? 18 : 1,
            keyboardType: switch (e.valueType) {
              ConfigValueType.integer => const TextInputType.numberWithOptions(signed: true),
              ConfigValueType.decimal => const TextInputType.numberWithOptions(signed: true, decimal: true),
              _ => multi ? TextInputType.multiline : TextInputType.text,
            },
            style: multi ? const TextStyle(fontFamily: 'monospace', fontSize: 13) : null,
            decoration: InputDecoration(
              labelText: e.valueType == ConfigValueType.stringList ? 'Items, one per line' : 'Value',
              border: const OutlineInputBorder(),
              errorText: check.error,
              errorMaxLines: 3,
            ),
            onChanged: (_) => setState(() {}),
          ),
      ]);
    },
    validate: () {
      final r = parseConfigValue(currentText(), e.valueType);
      if (!r.isOk) return {'value': r.error!};
      if (configValuesEqual(r.value, e.value)) return {'value': 'No change to save'};
      parsed = r.value;
      return const {};
    },
    confirmTitle: 'Save ${e.key}?',
    confirmMessage: 'This changes live behaviour for every user within about 15 seconds.',
    confirmDetails: () => DiffView(before: before, after: configValueToText(parsed, e.valueType)),
    save: () => api.update(e.key, parsed, e.version),
    successMessage: 'Saved ${e.key}',
  );
}

Future<void> showConfigHistory(BuildContext context, ConfigEntry e) {
  return showDialog<void>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: Text('History of ${e.key}'),
      content: SizedBox(
        width: 640,
        child: Consumer(builder: (ctx, ref, _) {
          final value = ref.watch(configHistoryProvider(e.key));
          return AsyncBody<List<ConfigChange>>(
            value: value,
            onRetry: () => ref.invalidate(configHistoryProvider(e.key)),
            data: (items) => items.isEmpty
                ? const Text('No changes recorded.')
                : ListView(shrinkWrap: true, children: [
                    for (final h in items)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 12),
                        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          Text('${formatDateTime(h.changedAt)}  ·  ${h.changedBy?.name ?? 'system'}'),
                          const SizedBox(height: 4),
                          DiffView(
                            before: configValueToText(h.oldValue, e.valueType),
                            after: configValueToText(h.newValue, e.valueType),
                          ),
                        ]),
                      ),
                  ]),
          );
        }),
      ),
      actions: [TextButton(onPressed: () => Navigator.of(ctx).pop(), child: const Text('Close'))],
    ),
  );
}

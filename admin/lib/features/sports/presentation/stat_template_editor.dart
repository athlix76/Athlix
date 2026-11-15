import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/widgets/common.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../application/sports_providers.dart';
import '../data/sports_api.dart';

final _keyRe = RegExp(r'^[a-z][a-z0-9_]{0,39}$');

class _FieldRow {
  _FieldRow(StatField? f)
      : key = TextEditingController(text: f?.key ?? ''),
        label = TextEditingController(text: f?.label ?? ''),
        unit = TextEditingController(text: f?.unit ?? ''),
        type = f?.type ?? 'int';

  final TextEditingController key;
  final TextEditingController label;
  final TextEditingController unit;
  String type;

  StatField toField() => StatField(
        key: key.text.trim(),
        label: label.text.trim(),
        type: type,
        unit: unit.text.trim().isEmpty ? null : unit.text.trim(),
      );
}

/// Create or edit a stat template. Changing the fields of an existing
/// template bumps its version on the server; old stat entries keep theirs.
Future<bool> editStatTemplate(BuildContext context, WidgetRef ref, AdminSport sport, StatTemplate? t) async {
  final api = ref.read(sportsApiProvider);
  final name = TextEditingController(text: t?.name ?? '');
  int? positionId = t?.positionId;
  var active = t?.active ?? true;
  final rows = [for (final f in t?.fields ?? const <StatField>[]) _FieldRow(f)];
  if (rows.isEmpty) rows.add(_FieldRow(null));
  String fieldsText(List<StatField> fs) => fs.map((f) => '${f.key}: ${f.label} [${f.type}${f.unit != null ? ', ${f.unit}' : ''}]').join('\n');

  final ok = await showEditorDialog(
    context,
    title: t == null ? 'Add stat template to ${sport.name}' : 'Edit ${t.name} (v${t.version})',
    width: 760,
    builder: (_, setState) => formColumn([
      FormText(label: 'Name', controller: name),
      FormDropdown<int>(
        label: 'Position',
        value: positionId,
        nullLabel: 'All positions',
        items: [for (final p in sport.positions) (p.id, p.name)],
        onChanged: t == null ? (v) => setState(() => positionId = v) : null,
      ),
      if (t != null) SwitchListTile(title: const Text('Active'), value: active, onChanged: (v) => setState(() => active = v)),
      Text('Fields', style: Theme.of(context).textTheme.titleSmall),
      for (var i = 0; i < rows.length; i++)
        Row(children: [
          Expanded(flex: 3, child: FormText(label: 'Key', controller: rows[i].key)),
          const SizedBox(width: 8),
          Expanded(flex: 4, child: FormText(label: 'Label', controller: rows[i].label)),
          const SizedBox(width: 8),
          Expanded(
            flex: 3,
            child: FormDropdown<String>(
              label: 'Type',
              value: rows[i].type,
              items: [for (final ty in statFieldTypes) (ty, ty)],
              onChanged: (v) => setState(() => rows[i].type = v ?? 'int'),
            ),
          ),
          const SizedBox(width: 8),
          Expanded(flex: 2, child: FormText(label: 'Unit', controller: rows[i].unit)),
          IconButton(
            tooltip: 'Remove field',
            icon: const Icon(Icons.close),
            onPressed: rows.length == 1 ? null : () => setState(() => rows.removeAt(i)),
          ),
        ]),
      Align(
        alignment: Alignment.centerLeft,
        child: TextButton.icon(
          onPressed: () => setState(() => rows.add(_FieldRow(null))),
          icon: const Icon(Icons.add),
          label: const Text('Add field'),
        ),
      ),
    ]),
    validate: () {
      final e = <String, String>{};
      if (name.text.trim().isEmpty) e['name'] = 'is required';
      final seen = <String>{};
      for (var i = 0; i < rows.length; i++) {
        final k = rows[i].key.text.trim();
        if (!_keyRe.hasMatch(k)) e['field ${i + 1} key'] = 'lower case letters, digits and _';
        if (!seen.add(k)) e['field ${i + 1} key'] = 'duplicate key "$k"';
        if (rows[i].label.text.trim().isEmpty) e['field ${i + 1} label'] = 'is required';
      }
      return e;
    },
    confirmMessage: t == null
        ? 'Add this stat template? Players and coaches can start recording these stats.'
        : 'Save the template? If the fields changed, the server creates a new version; stats already recorded keep the old one.',
    confirmDetails: () => t == null ? null : DiffView(before: fieldsText(t.fields), after: fieldsText([for (final r in rows) r.toField()])),
    save: () {
      final fields = [for (final r in rows) r.toField()];
      if (t == null) return api.createStatTemplate(sport.id, positionId: positionId, name: name.text.trim(), fields: fields);
      final changed = fieldsText(fields) != fieldsText(t.fields);
      return api.updateStatTemplate(t.id, {
        'name': name.text.trim(),
        'active': active,
        if (changed) 'fields': [for (final f in fields) f.toJson()],
      });
    },
  );
  return ok;
}

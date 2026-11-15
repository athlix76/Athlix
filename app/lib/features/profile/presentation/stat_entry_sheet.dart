import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/models/sport.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../application/profile_controller.dart';
import '../data/profile_models.dart';
import 'stat_format.dart';

/// Add or edit one stat entry. The inputs are built from the chosen template's
/// fields (sports config); nothing here knows a stat key.
Future<void> showStatEntrySheet(BuildContext context, {StatEntry? entry, int? sportId, int? templateId}) {
  return showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    showDragHandle: true,
    builder: (_) => _StatEntrySheet(
      entry: entry,
      sportId: entry?.sportId ?? sportId,
      templateId: entry?.templateId ?? templateId,
    ),
  );
}

class _StatEntrySheet extends ConsumerStatefulWidget {
  const _StatEntrySheet({this.entry, this.sportId, this.templateId});

  final StatEntry? entry;
  final int? sportId;
  final int? templateId;

  @override
  ConsumerState<_StatEntrySheet> createState() => _StatEntrySheetState();
}

class _StatEntrySheetState extends ConsumerState<_StatEntrySheet> {
  late int? _sportId = widget.sportId;
  late int? _templateId = widget.templateId;
  late final _season = TextEditingController(text: widget.entry?.season ?? '');
  final Map<String, TextEditingController> _fields = {};

  bool get _editing => widget.entry != null;

  @override
  void dispose() {
    _season.dispose();
    for (final c in _fields.values) {
      c.dispose();
    }
    super.dispose();
  }

  TextEditingController _controllerFor(StatField f) => _fields.putIfAbsent(
        f.key,
        () => TextEditingController(text: statInputText(f, widget.entry?.stats[f.key])),
      );

  void _selectTemplate(int? id) {
    for (final c in _fields.values) {
      c.dispose();
    }
    _fields.clear();
    setState(() => _templateId = id);
  }

  Future<void> _save(StatTemplate template) async {
    final season = _season.text.trim();
    if (season.isEmpty) {
      showInfoSnack(context, 'Enter the season.');
      return;
    }
    final stats = <String, Object>{};
    try {
      for (final f in template.fields) {
        final v = parseStatInput(f, _controllerFor(f).text);
        if (v != null) stats[f.key] = v;
      }
    } on FormatException catch (e) {
      showInfoSnack(context, e.message);
      return;
    }
    if (stats.isEmpty) {
      showInfoSnack(context, 'Enter at least one value.');
      return;
    }
    await ref.read(profileControllerProvider).saveStat(
          StatInput(sportId: _sportId!, templateId: template.id, season: season, stats: stats),
          id: widget.entry?.id,
        );
    if (mounted) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final sports = ref.watch(sportsProvider).where((s) => s.statTemplates.isNotEmpty).toList();
    Sport? sport;
    for (final s in sports) {
      if (s.id == _sportId) sport = s;
    }
    final templates = sport?.statTemplates ?? const <StatTemplate>[];
    StatTemplate? template;
    for (final t in templates) {
      if (t.id == _templateId) template = t;
    }
    if (template == null && templates.length == 1 && !_editing) template = templates.first;

    const gap = SizedBox(height: 12);
    final bottom = MediaQuery.of(context).viewInsets.bottom;
    return Padding(
      padding: EdgeInsets.fromLTRB(16, 0, 16, 16 + bottom),
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(_editing ? 'Edit stats' : 'Add stats', style: Theme.of(context).textTheme.titleLarge),
            gap,
            DropdownButtonFormField<int>(
              value: sport?.id,
              isExpanded: true,
              decoration: const InputDecoration(labelText: 'Sport'),
              items: [for (final s in sports) DropdownMenuItem(value: s.id, child: Text(s.name))],
              onChanged: _editing
                  ? null
                  : (v) {
                      _sportId = v;
                      _selectTemplate(null);
                    },
            ),
            if (templates.length > 1) ...[
              gap,
              DropdownButtonFormField<int>(
                value: template?.id,
                isExpanded: true,
                decoration: const InputDecoration(labelText: 'Stats template'),
                items: [for (final t in templates) DropdownMenuItem(value: t.id, child: Text(t.name))],
                onChanged: _editing ? null : _selectTemplate,
              ),
            ],
            gap,
            TextField(
              controller: _season,
              maxLength: 20,
              decoration: const InputDecoration(labelText: 'Season', hintText: 'e.g. 2025-26'),
            ),
            if (_editing && template == null)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 12),
                child: Text('This stats template is no longer available, so the entry cannot be edited.'),
              ),
            if (template != null)
              for (final f in template.fields) ...[
                gap,
                TextField(
                  controller: _controllerFor(f),
                  keyboardType: switch (f.type) {
                    'int' => TextInputType.number,
                    'text' => TextInputType.text,
                    _ => const TextInputType.numberWithOptions(decimal: true),
                  },
                  inputFormatters: f.type == 'int' ? [FilteringTextInputFormatter.digitsOnly] : null,
                  decoration: InputDecoration(labelText: statLabel(f)),
                ),
              ],
            const SizedBox(height: 20),
            BusyButton(label: 'Save', onPressed: template == null ? null : () => _save(template!)),
          ],
        ),
      ),
    );
  }
}

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/cursor_list.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../application/districts_providers.dart';
import '../data/districts_api.dart';
import 'aliases_field.dart';

/// Districts used by location identities and event eligibility. Districts are
/// deactivated, never deleted: existing identities and events keep them.
class DistrictsScreen extends ConsumerStatefulWidget {
  const DistrictsScreen({super.key});

  @override
  ConsumerState<DistrictsScreen> createState() => _DistrictsScreenState();
}

class _DistrictsScreenState extends ConsumerState<DistrictsScreen> {
  final _listKey = GlobalKey<CursorListState<AdminDistrict>>();
  int? _stateId;
  final _q = TextEditingController();
  bool _includeInactive = true;

  @override
  void dispose() {
    _q.dispose();
    super.dispose();
  }

  void _apply() => _listKey.currentState?.reload();

  Future<void> _edit(AdminDistrict? d) async {
    final api = ref.read(districtsApiProvider);
    final states = ref.read(adminStatesProvider).valueOrNull ?? const <AdminState>[];
    var stateId = d?.stateId ?? _stateId;
    final name = TextEditingController(text: d?.name ?? '');
    final lgd = TextEditingController(text: d?.lgdCode == null ? '' : '${d!.lgdCode}');
    var aliases = [...?d?.aliases];
    var active = d?.active ?? true;
    String text(String n, String l, List<String> a, bool act) => 'name: $n\nlgdCode: ${l.isEmpty ? '-' : l}\naliases: ${a.join(', ')}\nactive: $act';

    final ok = await showEditorDialog(
      context,
      title: d == null ? 'Add district' : 'Edit ${d.name}',
      builder: (_, setState) => formColumn([
        DropdownButtonFormField<int>(
          value: states.any((x) => x.id == stateId) ? stateId : null,
          isExpanded: true,
          decoration: const InputDecoration(labelText: 'State', helperText: 'States are seeded by migration', border: OutlineInputBorder()),
          items: [
            for (final x in states)
              if (x.active || x.id == stateId) DropdownMenuItem<int>(value: x.id, child: Text(x.active ? x.name : '${x.name} (inactive)')),
          ],
          onChanged: d == null ? (v) => setState(() => stateId = v) : null,
        ),
        FormText(label: 'Name (official)', controller: name),
        FormText(label: 'LGD code (optional)', controller: lgd),
        AliasesField(aliases: aliases, onChanged: (v) => setState(() => aliases = v)),
        if (d != null)
          SwitchListTile(
            title: const Text('Active'),
            subtitle: const Text('Inactive districts are hidden from pickers. Existing identities and events keep them.'),
            value: active,
            onChanged: (v) => setState(() => active = v),
          ),
      ]),
      validate: () => {
        if (d == null && (stateId ?? 0) <= 0) 'stateId': 'pick a state',
        if (name.text.trim().isEmpty) 'name': 'is required',
        if (lgd.text.trim().isNotEmpty && int.tryParse(lgd.text.trim()) == null) 'lgdCode': 'digits only, or empty',
      },
      confirmMessage: d == null
          ? 'Add this district? It appears in the app pickers straight away.'
          : 'Save the district? Place names used by eligibility refresh within about 10 minutes.',
      confirmDetails: () => DiffView(
        before: d == null ? '' : text(d.name, d.lgdCode == null ? '' : '${d.lgdCode}', d.aliases, d.active),
        after: text(name.text.trim(), lgd.text.trim(), aliases, active),
      ),
      save: () {
        final code = lgd.text.trim().isEmpty ? null : int.parse(lgd.text.trim());
        if (d == null) {
          return api.create(stateId: stateId!, name: name.text.trim(), lgdCode: code, aliases: aliases);
        }
        return api.update(d.id, name: name.text.trim(), lgdCode: code, aliases: aliases, active: active);
      },
    );
    if (ok) _apply();
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(districtsApiProvider);
    final canWrite = ref.watch(sessionProvider).can(Perm.geoWrite);
    final states = ref.watch(adminStatesProvider).valueOrNull ?? const <AdminState>[];
    final stateNames = {for (final x in states) x.id: x.name};
    return PageScaffold(
      title: 'Districts',
      subtitle: 'Bulk loads use the LGD CSV import in the backend admin CLI. Use this screen for fixes.',
      actions: [
        if (canWrite) FilledButton.icon(onPressed: () => _edit(null), icon: const Icon(Icons.add), label: const Text('Add district')),
      ],
      filters: Wrap(spacing: 12, runSpacing: 12, crossAxisAlignment: WrapCrossAlignment.center, children: [
        SizedBox(
          width: 220,
          child: DropdownButtonFormField<int?>(
            value: states.any((x) => x.id == _stateId) ? _stateId : null,
            isExpanded: true,
            decoration: const InputDecoration(labelText: 'State', border: OutlineInputBorder(), isDense: true),
            items: [
              const DropdownMenuItem<int?>(value: null, child: Text('Any')),
              for (final x in states) DropdownMenuItem<int?>(value: x.id, child: Text(x.name)),
            ],
            onChanged: (v) {
              setState(() => _stateId = v);
              _apply();
            },
          ),
        ),
        FilterTextField(label: 'Name', controller: _q, onSubmitted: (_) => _apply()),
        FilterChip(
          label: const Text('Include inactive'),
          selected: _includeInactive,
          onSelected: (v) {
            setState(() => _includeInactive = v);
            _apply();
          },
        ),
      ]),
      body: CursorList<AdminDistrict>(
        key: _listKey,
        load: (cursor) => api.list(
          stateId: _stateId,
          q: _q.text.trim(),
          includeInactive: _includeInactive,
          cursor: cursor,
        ),
        emptyText: 'No districts match.',
        itemBuilder: (context, d) => ListTile(
          title: Text('${d.name}  #${d.id}'),
          subtitle: Text('${stateNames[d.stateId] ?? 'state ${d.stateId}'}  ·  LGD ${d.lgdCode ?? '-'}'
              '${d.aliases.isEmpty ? '' : '  ·  also: ${d.aliases.join(', ')}'}'),
          leading: StatusChip(d.active ? 'active' : 'disabled'),
          trailing: canWrite ? IconButton(tooltip: 'Edit', icon: const Icon(Icons.edit_outlined), onPressed: () => _edit(d)) : null,
        ),
      ),
    );
  }
}

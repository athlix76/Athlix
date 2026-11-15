import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../application/sports_providers.dart';
import '../data/sports_api.dart';
import 'stat_template_editor.dart';

final _codeRe = RegExp(r'^[a-z][a-z0-9_]{1,39}$');

/// Sports, their positions and stat templates. Sports are never deleted;
/// they are deactivated so existing profiles and events keep their links.
class SportsScreen extends ConsumerWidget {
  const SportsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(adminSportsProvider);
    final canWrite = ref.watch(sessionProvider).can(Perm.sportsWrite);
    void reload() => ref.invalidate(adminSportsProvider);
    return PageScaffold(
      title: 'Sports, positions and stat templates',
      actions: [
        IconButton(tooltip: 'Reload', icon: const Icon(Icons.refresh), onPressed: reload),
        if (canWrite)
          FilledButton.icon(
            onPressed: () async {
              if (await _editSport(context, ref, null)) reload();
            },
            icon: const Icon(Icons.add),
            label: const Text('Add sport'),
          ),
      ],
      body: AsyncBody<List<AdminSport>>(
        value: value,
        onRetry: reload,
        data: (sports) => ListView(children: [
          for (final s in sports)
            Card(
              child: ExpansionTile(
                title: Text('${s.name}  (${s.code})'),
                subtitle: Text('${s.isTeamSport ? 'team' : 'individual'}  ·  order ${s.sortOrder}  ·  '
                    '${s.positions.length} positions  ·  ${s.statTemplates.length} stat templates'),
                leading: StatusChip(s.active ? 'active' : 'disabled'),
                trailing: canWrite
                    ? IconButton(
                        tooltip: 'Edit sport',
                        icon: const Icon(Icons.edit_outlined),
                        onPressed: () async {
                          if (await _editSport(context, ref, s)) reload();
                        },
                      )
                    : null,
                childrenPadding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
                expandedCrossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Row(children: [
                    Expanded(child: Text('Positions', style: Theme.of(context).textTheme.titleSmall)),
                    if (canWrite)
                      TextButton.icon(
                        onPressed: () async {
                          if (await _editPosition(context, ref, s, null)) reload();
                        },
                        icon: const Icon(Icons.add),
                        label: const Text('Add position'),
                      ),
                  ]),
                  for (final p in s.positions)
                    ListTile(
                      dense: true,
                      title: Text('${p.name}  (${p.code})'),
                      subtitle: Text('applies to ${p.appliesTo}  ·  order ${p.sortOrder}'),
                      leading: StatusChip(p.active ? 'active' : 'disabled'),
                      trailing: canWrite
                          ? IconButton(
                              icon: const Icon(Icons.edit_outlined),
                              onPressed: () async {
                                if (await _editPosition(context, ref, s, p)) reload();
                              },
                            )
                          : null,
                    ),
                  const Divider(),
                  Row(children: [
                    Expanded(child: Text('Stat templates', style: Theme.of(context).textTheme.titleSmall)),
                    if (canWrite)
                      TextButton.icon(
                        onPressed: () async {
                          if (await editStatTemplate(context, ref, s, null)) reload();
                        },
                        icon: const Icon(Icons.add),
                        label: const Text('Add template'),
                      ),
                  ]),
                  for (final t in s.statTemplates)
                    ListTile(
                      dense: true,
                      title: Text('${t.name}  · v${t.version}'),
                      subtitle: Text('${_positionName(s, t.positionId)}  ·  '
                          '${t.fields.map((f) => '${f.label} (${f.type}${f.unit != null ? ', ${f.unit}' : ''})').join(', ')}'),
                      leading: StatusChip(t.active ? 'active' : 'disabled'),
                      trailing: canWrite
                          ? IconButton(
                              icon: const Icon(Icons.edit_outlined),
                              onPressed: () async {
                                if (await editStatTemplate(context, ref, s, t)) reload();
                              },
                            )
                          : null,
                    ),
                ],
              ),
            ),
        ]),
      ),
    );
  }
}

String _positionName(AdminSport s, int? positionId) {
  if (positionId == null) return 'all positions';
  for (final p in s.positions) {
    if (p.id == positionId) return p.name;
  }
  return 'position #$positionId';
}

Future<bool> _editSport(BuildContext context, WidgetRef ref, AdminSport? s) async {
  final api = ref.read(sportsApiProvider);
  final code = TextEditingController(text: s?.code ?? '');
  final name = TextEditingController(text: s?.name ?? '');
  final order = TextEditingController(text: '${s?.sortOrder ?? 0}');
  var team = s?.isTeamSport ?? false;
  var active = s?.active ?? true;
  final ok = await showEditorDialog(
    context,
    title: s == null ? 'Add sport' : 'Edit ${s.name}',
    builder: (_, setState) => formColumn([
      FormText(label: 'Code (lower_snake, cannot change later)', controller: code, enabled: s == null),
      FormText(label: 'Name', controller: name),
      FormText(label: 'Sort order', controller: order, keyboardType: TextInputType.number),
      SwitchListTile(title: const Text('Team sport'), value: team, onChanged: (v) => setState(() => team = v)),
      if (s != null)
        SwitchListTile(
          title: const Text('Active'),
          subtitle: const Text('Inactive sports are hidden from the app but kept on existing records.'),
          value: active,
          onChanged: (v) => setState(() => active = v),
        ),
    ]),
    validate: () => {
      if (s == null && !_codeRe.hasMatch(code.text.trim())) 'code': 'lower case letters, digits and _ (2 to 40)',
      if (name.text.trim().isEmpty) 'name': 'is required',
      if (int.tryParse(order.text.trim()) == null) 'sortOrder': 'must be a whole number',
    },
    confirmMessage: s == null ? 'Add this sport? It shows in the app straight away.' : 'Save changes to ${s.name}?',
    save: () {
      final sortOrder = int.parse(order.text.trim());
      if (s == null) {
        return api.createSport(code: code.text.trim(), name: name.text.trim(), isTeamSport: team, sortOrder: sortOrder);
      }
      return api.updateSport(s.id, {'name': name.text.trim(), 'isTeamSport': team, 'sortOrder': sortOrder, 'active': active});
    },
  );
  return ok;
}

Future<bool> _editPosition(BuildContext context, WidgetRef ref, AdminSport sport, Position? p) async {
  final api = ref.read(sportsApiProvider);
  final code = TextEditingController(text: p?.code ?? '');
  final name = TextEditingController(text: p?.name ?? '');
  final order = TextEditingController(text: '${p?.sortOrder ?? 0}');
  var appliesTo = p?.appliesTo ?? 'both';
  var active = p?.active ?? true;
  final ok = await showEditorDialog(
    context,
    title: p == null ? 'Add position to ${sport.name}' : 'Edit position ${p.name}',
    builder: (_, setState) => formColumn([
      FormText(label: 'Code (lower_snake, cannot change later)', controller: code, enabled: p == null),
      FormText(label: 'Name', controller: name),
      FormDropdown<String>(
        label: 'Applies to',
        value: appliesTo,
        items: [for (final a in positionAppliesTo) (a, a)],
        onChanged: (v) => setState(() => appliesTo = v ?? 'both'),
      ),
      FormText(label: 'Sort order', controller: order, keyboardType: TextInputType.number),
      if (p != null) SwitchListTile(title: const Text('Active'), value: active, onChanged: (v) => setState(() => active = v)),
    ]),
    validate: () => {
      if (p == null && !_codeRe.hasMatch(code.text.trim())) 'code': 'lower case letters, digits and _ (2 to 40)',
      if (name.text.trim().isEmpty) 'name': 'is required',
      if (int.tryParse(order.text.trim()) == null) 'sortOrder': 'must be a whole number',
    },
    confirmMessage: p == null ? 'Add this position to ${sport.name}?' : 'Save changes to ${p.name}?',
    save: () {
      final sortOrder = int.parse(order.text.trim());
      if (p == null) {
        return api.createPosition(sport.id, code: code.text.trim(), name: name.text.trim(), appliesTo: appliesTo, sortOrder: sortOrder);
      }
      return api.updatePosition(p.id, {'name': name.text.trim(), 'appliesTo': appliesTo, 'sortOrder': sortOrder, 'active': active});
    },
  );
  return ok;
}

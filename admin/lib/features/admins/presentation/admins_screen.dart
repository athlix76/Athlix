import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/auth/admin_me.dart';
import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../application/admins_providers.dart';
import '../data/admins_api.dart';
import 'one_time_credentials_dialog.dart';

/// Admin users and roles (admins.manage).
class AdminsScreen extends ConsumerWidget {
  const AdminsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return const DefaultTabController(
      length: 2,
      child: PageScaffold(
        title: 'Admins and roles',
        filters: TabBar(isScrollable: true, tabs: [Tab(text: 'Admins'), Tab(text: 'Roles')]),
        body: TabBarView(children: [_AdminsTab(), _RolesTab()]),
      ),
    );
  }
}

class _AdminsTab extends ConsumerWidget {
  const _AdminsTab();

  Future<void> _create(BuildContext context, WidgetRef ref, List<AdminRole> roles) async {
    final api = ref.read(adminsApiProvider);
    final email = TextEditingController();
    final name = TextEditingController();
    int? roleId = roles.isEmpty ? null : roles.first.id;
    (AdminMe, OneTimeCredentials)? created;
    final saved = await showEditorDialog(
      context,
      title: 'New admin',
      builder: (ctx, setState) => formColumn([
        FormText(label: 'Work email', controller: email, keyboardType: TextInputType.emailAddress),
        FormText(label: 'Name', controller: name),
        FormDropdown<int>(
          label: 'Role',
          value: roleId,
          items: [for (final r in roles) (r.id, r.name)],
          onChanged: (v) => setState(() => roleId = v),
        ),
      ]),
      validate: () => {
        if (!RegExp(r'^[^@\s]+@[^@\s]+\.[^@\s]+$').hasMatch(email.text.trim())) 'email': 'Enter a valid email',
        if (name.text.trim().isEmpty) 'name': 'Required',
        if (roleId == null) 'roleId': 'Choose a role',
      },
      confirmMessage: 'Create this admin? A temporary password and TOTP setup are shown once on the next screen.',
      successMessage: null,
      save: () async => created = await api.create(email: email.text.trim(), name: name.text.trim(), roleId: roleId!),
    );
    final c = created;
    if (saved && c != null && context.mounted) {
      ref.invalidate(adminUsersProvider);
      await showOneTimeCredentials(context, adminName: c.$1.name, creds: c.$2);
    }
  }

  Future<void> _edit(BuildContext context, WidgetRef ref, AdminMe a, List<AdminRole> roles, {required bool isSelf}) async {
    final api = ref.read(adminsApiProvider);
    final name = TextEditingController(text: a.name);
    var roleId = a.roleId;
    var status = a.status;
    final saved = await showEditorDialog(
      context,
      title: 'Edit ${a.email}',
      builder: (ctx, setState) => formColumn([
        FormText(label: 'Name', controller: name),
        FormDropdown<int>(
          label: 'Role',
          value: roleId,
          items: [
            for (final r in roles) (r.id, r.name),
            if (!roles.any((r) => r.id == a.roleId)) (a.roleId, a.roleName),
          ],
          onChanged: (v) => setState(() => roleId = v ?? roleId),
        ),
        FormDropdown<String>(
          label: isSelf ? 'Status (you cannot disable yourself)' : 'Status',
          value: status,
          items: [for (final s in {...adminStatuses, a.status}) if (!isSelf || s == 'active' || s == a.status) (s, s)],
          onChanged: (v) => setState(() => status = v ?? status),
        ),
      ]),
      validate: () => {
        if (name.text.trim().isEmpty) 'name': 'Required',
        if (name.text.trim() == a.name && roleId == a.roleId && status == a.status) 'name': 'No change',
      },
      confirmMessage: 'Save changes to this admin? A role or status change applies on their next request; disabling ends their sessions.',
      save: () => api.update(
        a.id,
        name: name.text.trim() == a.name ? null : name.text.trim(),
        roleId: roleId == a.roleId ? null : roleId,
        status: status == a.status ? null : status,
      ),
    );
    if (saved) ref.invalidate(adminUsersProvider);
  }

  Future<void> _reset(BuildContext context, WidgetRef ref, AdminMe a) async {
    final creds = await confirmAction<OneTimeCredentials>(
      context,
      title: 'Reset credentials for ${a.name}?',
      message: 'Their current password and authenticator stop working at once and their sessions end. '
          'New credentials are shown once on the next screen.',
      confirmLabel: 'Reset',
      destructive: true,
      run: (_) => ref.read(adminsApiProvider).resetCredentials(a.id),
    );
    if (creds != null && context.mounted) await showOneTimeCredentials(context, adminName: a.name, creds: creds);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final admins = ref.watch(adminUsersProvider);
    final roles = ref.watch(adminRolesProvider).valueOrNull ?? const <AdminRole>[];
    final myId = ref.watch(sessionProvider).me?.id;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Padding(
        padding: const EdgeInsets.all(12),
        child: Align(
          alignment: Alignment.centerRight,
          child: FilledButton.icon(
            onPressed: () => _create(context, ref, roles),
            icon: const Icon(Icons.person_add),
            label: const Text('New admin'),
          ),
        ),
      ),
      Expanded(
        child: AsyncBody<List<AdminMe>>(
          value: admins,
          onRetry: () => ref.invalidate(adminUsersProvider),
          data: (list) => list.isEmpty
              ? const Center(child: Text('No admins.'))
              : ListView.separated(
                  itemCount: list.length,
                  separatorBuilder: (_, __) => const Divider(height: 1),
                  itemBuilder: (context, i) {
                    final a = list[i];
                    final isSelf = a.id == myId;
                    return ListTile(
                      title: Text('${a.name}${isSelf ? ' (you)' : ''}  ·  ${a.email}'),
                      subtitle: Text('role ${a.roleName}  ·  last sign-in ${formatDateTime(a.lastLoginAt)}'),
                      trailing: Wrap(spacing: 4, crossAxisAlignment: WrapCrossAlignment.center, children: [
                        StatusChip(a.status),
                        IconButton(tooltip: 'Edit', icon: const Icon(Icons.edit), onPressed: () => _edit(context, ref, a, roles, isSelf: isSelf)),
                        if (!isSelf)
                          IconButton(tooltip: 'Reset password and TOTP', icon: const Icon(Icons.key), onPressed: () => _reset(context, ref, a)),
                      ]),
                    );
                  },
                ),
        ),
      ),
    ]);
  }
}

class _RolesTab extends ConsumerWidget {
  const _RolesTab();

  Future<void> _edit(BuildContext context, WidgetRef ref, AdminRole? existing) async {
    final api = ref.read(adminsApiProvider);
    final name = TextEditingController(text: existing?.name ?? '');
    final description = TextEditingController(text: existing?.description ?? '');
    final perms = <String>{...?existing?.permissions};
    final saved = await showEditorDialog(
      context,
      title: existing == null ? 'New role' : 'Edit role ${existing.name}',
      width: 640,
      builder: (ctx, setState) => formColumn([
        FormText(label: 'Name (cannot change later)', controller: name, enabled: existing == null),
        FormText(label: 'Description', controller: description),
        if (perms.contains(Perm.all))
          Text('This role has "*" (every permission). It is kept as is.', style: TextStyle(color: Theme.of(ctx).colorScheme.error)),
        Wrap(spacing: 4, runSpacing: 0, children: [
          for (final p in Perm.values)
            SizedBox(
              width: 280,
              child: CheckboxListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                controlAffinity: ListTileControlAffinity.leading,
                title: Text(p, style: const TextStyle(fontFamily: 'monospace')),
                value: perms.contains(p),
                onChanged: (on) => setState(() => on == true ? perms.add(p) : perms.remove(p)),
              ),
            ),
        ]),
      ]),
      validate: () => {
        if (existing == null && name.text.trim().isEmpty) 'name': 'Required',
        if (perms.isEmpty) 'permissions': 'Choose at least one permission',
      },
      confirmMessage: existing == null
          ? 'Create this role?'
          : 'Save this role? Every admin with it gets the new permissions on their next request.',
      confirmDetails: existing == null
          ? null
          : () => DiffView(before: (existing.permissions.toList()..sort()).join('\n'), after: (perms.toList()..sort()).join('\n')),
      save: () => existing == null
          ? api.createRole(name: name.text.trim(), description: description.text.trim(), permissions: perms.toList())
          : api.updateRole(existing.id, description: description.text.trim(), permissions: perms.toList()),
    );
    if (saved) ref.invalidate(adminRolesProvider);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final roles = ref.watch(adminRolesProvider);
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Padding(
        padding: const EdgeInsets.all(12),
        child: Align(
          alignment: Alignment.centerRight,
          child: FilledButton.icon(onPressed: () => _edit(context, ref, null), icon: const Icon(Icons.add), label: const Text('New role')),
        ),
      ),
      Expanded(
        child: AsyncBody<List<AdminRole>>(
          value: roles,
          onRetry: () => ref.invalidate(adminRolesProvider),
          data: (list) => ListView.separated(
            itemCount: list.length,
            separatorBuilder: (_, __) => const Divider(height: 1),
            itemBuilder: (context, i) {
              final r = list[i];
              return ListTile(
                title: Text(r.name),
                subtitle: Text('${r.description}\n${r.permissions.join(', ')}'),
                isThreeLine: true,
                trailing: IconButton(tooltip: 'Edit', icon: const Icon(Icons.edit), onPressed: () => _edit(context, ref, r)),
              );
            },
          ),
        ),
      ),
    ]);
  }
}

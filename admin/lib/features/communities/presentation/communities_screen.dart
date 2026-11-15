import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/format/format.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/cursor_list.dart';
import '../application/communities_providers.dart';
import '../data/communities_api.dart';

class CommunitiesScreen extends ConsumerStatefulWidget {
  const CommunitiesScreen({super.key});

  @override
  ConsumerState<CommunitiesScreen> createState() => _CommunitiesScreenState();
}

class _CommunitiesScreenState extends ConsumerState<CommunitiesScreen> {
  final _listKey = GlobalKey<CursorListState<AdminCommunity>>();
  final _q = TextEditingController();
  String? _status;

  @override
  void dispose() {
    _q.dispose();
    super.dispose();
  }

  void _reload() => _listKey.currentState?.reload();

  Future<void> _remove(AdminCommunity c) async {
    final api = ref.read(communitiesApiProvider);
    final ok = await confirmAction<bool>(
      context,
      title: 'Remove community "${c.title}"?',
      message: 'The community and its group chat are closed for all ${c.memberCount} members. The owner is notified.',
      confirmLabel: 'Remove',
      destructive: true,
      reason: const ReasonSpec(hint: 'Shown to the owner and kept in the audit log'),
      run: (reason) async {
        await api.remove(c.id, reason!);
        return true;
      },
      successMessage: 'Community removed',
    );
    if (ok == true) _reload();
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(communitiesApiProvider);
    return PageScaffold(
      title: 'Communities',
      filters: Wrap(spacing: 12, runSpacing: 12, children: [
        FilterTextField(label: 'Search title', controller: _q, onSubmitted: (_) => _reload()),
        FilterDropdown(
          label: 'Status',
          value: _status,
          options: communityStatuses,
          onChanged: (v) {
            setState(() => _status = v);
            _reload();
          },
        ),
      ]),
      body: CursorList<AdminCommunity>(
        key: _listKey,
        load: (cursor) => api.list(q: _q.text.trim(), status: _status, cursor: cursor),
        emptyText: 'No communities.',
        itemBuilder: (context, c) => ListTile(
          title: Text('${c.title}  ·  ${c.memberCount} members'),
          subtitle: Text([
            if (c.sport != null) c.sport!.name,
            [c.city, c.state].whereType<String>().join(', '),
            '${c.visibility} · ${c.joinPolicy}',
            'owner ${c.owner?.label ?? '-'}',
            'created ${formatDate(c.createdAt)}',
          ].where((s) => s.isNotEmpty).join('  ·  ')),
          trailing: Wrap(spacing: 8, crossAxisAlignment: WrapCrossAlignment.center, children: [
            StatusChip(c.status),
            if (c.status != 'removed') OutlinedButton(onPressed: () => _remove(c), child: const Text('Remove')),
          ]),
        ),
      ),
    );
  }
}

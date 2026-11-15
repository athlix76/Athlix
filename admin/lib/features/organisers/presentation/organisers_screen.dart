import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/format.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/cursor_list.dart';
import '../application/organisers_providers.dart';
import '../data/organisers_api.dart';

class OrganisersScreen extends ConsumerStatefulWidget {
  const OrganisersScreen({super.key});

  @override
  ConsumerState<OrganisersScreen> createState() => _OrganisersScreenState();
}

class _OrganisersScreenState extends ConsumerState<OrganisersScreen> {
  final _listKey = GlobalKey<CursorListState<OrganiserApplicationRow>>();
  String? _status = 'submitted';

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(organisersApiProvider);
    return PageScaffold(
      title: 'Organiser applications',
      subtitle: 'Review submitted organiser verification and approve or reject it.',
      filters: Wrap(spacing: 12, children: [
        FilterDropdown(
          label: 'Status',
          value: _status,
          options: const ['submitted', 'approved', 'rejected', 'draft'],
          onChanged: (v) {
            setState(() => _status = v);
            _listKey.currentState?.reload();
          },
        ),
      ]),
      body: CursorList<OrganiserApplicationRow>(
        key: _listKey,
        load: (cursor) => api.list(_status, cursor),
        emptyText: 'No applications.',
        itemBuilder: (context, a) => ListTile(
          title: Text('${a.orgName ?? '(no organisation name)'}  ·  ${a.orgType ?? '-'}'),
          subtitle: Text('${a.user.label}  ·  submitted ${formatDateTime(a.submittedAt)}'),
          trailing: StatusChip(a.status),
          onTap: () => context.go('/organisers/${a.user.id}'),
        ),
      ),
    );
  }
}

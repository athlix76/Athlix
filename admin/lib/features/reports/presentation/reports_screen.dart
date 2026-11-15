import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/cursor_list.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../../moderation/presentation/conversation_messages_screen.dart';
import '../application/reports_providers.dart';
import '../data/reports_api.dart';

/// User reports. `underage_safety` reports are ordered first by the backend.
class ReportsScreen extends ConsumerStatefulWidget {
  const ReportsScreen({super.key});

  @override
  ConsumerState<ReportsScreen> createState() => _ReportsScreenState();
}

class _ReportsScreenState extends ConsumerState<ReportsScreen> {
  final _listKey = GlobalKey<CursorListState<AdminReport>>();
  String? _status = 'open';
  String? _targetType;
  String? _reason;

  void _reload() => _listKey.currentState?.reload();

  Future<void> _resolve(AdminReport r) async {
    final api = ref.read(reportsApiProvider);
    var action = reportResolveActions.first;
    final res = await confirmAction<AdminReport>(
      context,
      title: 'Resolve report #${r.id}',
      message: 'Choose what happens to the ${r.targetType}. Hide / remove / suspend are recorded as moderation actions.',
      confirmLabel: 'Resolve',
      details: StatefulBuilder(
        builder: (ctx, setState) => FormDropdown<String>(
          label: 'Action',
          value: action,
          items: [for (final a in reportResolveActions) (a, a.replaceAll('_', ' '))],
          onChanged: (v) => setState(() => action = v ?? action),
        ),
      ),
      reason: const ReasonSpec(label: 'Note', hint: 'Why (kept in the audit log)'),
      run: (note) => api.resolve(r.id, action, note),
      successMessage: 'Report resolved',
    );
    if (res != null) _reload();
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(reportsApiProvider);
    final session = ref.watch(sessionProvider);
    final canResolve = session.can(Perm.reportsResolve);
    final canReadMessages = session.can(Perm.moderationReadMessages);
    void filter(void Function() f) {
      setState(f);
      _reload();
    }

    return PageScaffold(
      title: 'Reports',
      filters: Wrap(spacing: 12, runSpacing: 12, children: [
        FilterDropdown(label: 'Status', value: _status, options: reportStatuses, onChanged: (v) => filter(() => _status = v)),
        FilterDropdown(label: 'Target', value: _targetType, options: reportTargetTypes, onChanged: (v) => filter(() => _targetType = v)),
        FilterDropdown(label: 'Reason', value: _reason, options: reportReasons, onChanged: (v) => filter(() => _reason = v)),
      ]),
      body: CursorList<AdminReport>(
        key: _listKey,
        load: (cursor) => api.list(ReportFilters(status: _status, targetType: _targetType, reason: _reason), cursor),
        emptyText: 'No reports.',
        itemBuilder: (context, r) => ListTile(
          leading: Icon(r.reason == 'underage_safety' ? Icons.child_care : Icons.flag_outlined,
              color: r.reason == 'underage_safety' ? Theme.of(context).colorScheme.error : null),
          title: Text('${r.reason.replaceAll('_', ' ')}  ·  ${r.targetType} #${r.targetId}'
              '${r.openReportsOnTarget > 1 ? '  ·  ${r.openReportsOnTarget} open reports' : ''}'),
          subtitle: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            if (r.previewTitle != null) Text(r.previewTitle!, style: const TextStyle(fontWeight: FontWeight.w600)),
            if (r.previewText != null) Text(r.previewText!, maxLines: 3, overflow: TextOverflow.ellipsis),
            if (r.details != null && r.details!.isNotEmpty) Text('Reporter says: ${r.details}'),
            Text([
              'by ${r.reporter?.label ?? 'unknown'}',
              if (r.ownerId != null) 'owner #${r.ownerId}',
              formatDateTime(r.createdAt),
              if (r.resolutionNote != null) 'note: ${r.resolutionNote}',
            ].join('  ·  ')),
          ]),
          isThreeLine: true,
          trailing: Wrap(spacing: 8, crossAxisAlignment: WrapCrossAlignment.center, children: [
            StatusChip(r.status),
            if (r.targetType == 'conversation' && canReadMessages)
              OutlinedButton(
                onPressed: () => openReportedConversation(context, r.targetId),
                child: const Text('Read messages'),
              ),
            if (r.targetType == 'user')
              TextButton(onPressed: () => context.go('/users/${r.targetId}'), child: const Text('User')),
            if (r.targetType == 'event')
              TextButton(onPressed: () => context.go('/events/${r.targetId}'), child: const Text('Event')),
            if (canResolve && r.status == 'open') FilledButton(onPressed: () => _resolve(r), child: const Text('Resolve')),
          ]),
        ),
      ),
    );
  }
}

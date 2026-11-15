import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/format/format.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/cursor_list.dart';
import '../application/audit_providers.dart';
import '../data/audit_api.dart';

/// Read-only list of every admin write and every sensitive read (proof views,
/// message reads, verified-name reveals). Newest first.
class AuditLogScreen extends ConsumerStatefulWidget {
  const AuditLogScreen({super.key});

  @override
  ConsumerState<AuditLogScreen> createState() => _AuditLogScreenState();
}

class _AuditLogScreenState extends ConsumerState<AuditLogScreen> {
  final _listKey = GlobalKey<CursorListState<AuditEntry>>();
  final _adminId = TextEditingController();
  final _action = TextEditingController();
  final _targetType = TextEditingController();
  final _targetId = TextEditingController();

  @override
  void dispose() {
    _adminId.dispose();
    _action.dispose();
    _targetType.dispose();
    _targetId.dispose();
    super.dispose();
  }

  AuditFilters get _filters => AuditFilters(
        adminId: int.tryParse(_adminId.text.trim()),
        action: _action.text.trim(),
        targetType: _targetType.text.trim(),
        targetId: _targetId.text.trim(),
      );

  void _reload() => _listKey.currentState?.reload();

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(auditApiProvider);
    return PageScaffold(
      title: 'Audit log',
      subtitle: 'Press Enter in any filter to apply it.',
      filters: Wrap(spacing: 12, runSpacing: 12, children: [
        FilterTextField(label: 'Admin id', controller: _adminId, width: 120, onSubmitted: (_) => _reload()),
        FilterTextField(label: 'Action (e.g. config.update)', controller: _action, onSubmitted: (_) => _reload()),
        FilterTextField(label: 'Target type', controller: _targetType, width: 180, onSubmitted: (_) => _reload()),
        FilterTextField(label: 'Target id', controller: _targetId, width: 160, onSubmitted: (_) => _reload()),
      ]),
      body: CursorList<AuditEntry>(
        key: _listKey,
        load: (cursor) => api.list(_filters, cursor: cursor),
        emptyText: 'No audit entries match.',
        itemBuilder: (context, e) => ExpansionTile(
          title: Text('${e.action}  ·  ${e.targetType}${e.targetId == null ? '' : ' #${e.targetId}'}'),
          subtitle: Text('${e.admin == null ? 'system' : '${e.admin!.name} (#${e.admin!.id})'}  ·  ${formatDateTime(e.createdAt)}'),
          childrenPadding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
          children: [
            if (e.details == null) const Align(alignment: Alignment.centerLeft, child: Text('No details.')) else JsonBlock(e.details),
          ],
        ),
      ),
    );
  }
}

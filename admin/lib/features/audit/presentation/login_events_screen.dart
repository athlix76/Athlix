import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/format/format.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/cursor_list.dart';
import '../application/audit_providers.dart';
import '../data/audit_api.dart';

/// Admin sign-in attempts (`GET /admin/login-events`, audit.read), newest
/// first. Failed attempts show the internal reason (bad password, locked,
/// unknown email …) that the login screen deliberately hides.
class LoginEventsScreen extends ConsumerStatefulWidget {
  const LoginEventsScreen({super.key});

  @override
  ConsumerState<LoginEventsScreen> createState() => _LoginEventsScreenState();
}

class _LoginEventsScreenState extends ConsumerState<LoginEventsScreen> {
  final _listKey = GlobalKey<CursorListState<LoginEvent>>();
  final _adminId = TextEditingController();
  final _email = TextEditingController();
  String _outcome = '';

  @override
  void dispose() {
    _adminId.dispose();
    _email.dispose();
    super.dispose();
  }

  LoginEventFilters get _filters => LoginEventFilters(
        adminId: int.tryParse(_adminId.text.trim()),
        email: _email.text.trim().isEmpty ? null : _email.text.trim(),
        outcome: _outcome.isEmpty ? null : _outcome,
      );

  void _reload() => _listKey.currentState?.reload();

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(auditApiProvider);
    return PageScaffold(
      title: 'Admin sign-ins',
      subtitle: 'Press Enter in a text filter to apply it.',
      filters: Wrap(spacing: 12, runSpacing: 12, crossAxisAlignment: WrapCrossAlignment.center, children: [
        FilterTextField(label: 'Admin id', controller: _adminId, width: 120, onSubmitted: (_) => _reload()),
        FilterTextField(label: 'Email tried', controller: _email, width: 260, onSubmitted: (_) => _reload()),
        SegmentedButton<String>(
          segments: const [
            ButtonSegment(value: '', label: Text('All')),
            ButtonSegment(value: 'success', label: Text('Success')),
            ButtonSegment(value: 'failure', label: Text('Failure')),
          ],
          selected: {_outcome},
          onSelectionChanged: (v) {
            setState(() => _outcome = v.first);
            _reload();
          },
        ),
      ]),
      body: CursorList<LoginEvent>(
        key: _listKey,
        load: (cursor) => api.loginEvents(_filters, cursor: cursor),
        emptyText: 'No sign-in attempts match.',
        itemBuilder: (context, e) {
          final who = e.admin == null ? 'Unknown email' : '${e.admin!.name} (#${e.admin!.id})';
          final where = [e.ip, e.userAgent].whereType<String>().where((s) => s.isNotEmpty).join('  ·  ');
          return ListTile(
            leading: Icon(
              e.success ? Icons.check_circle_outline : Icons.error_outline,
              color: e.success ? Colors.green : Theme.of(context).colorScheme.error,
            ),
            title: Text(e.success ? who : '$who  ·  ${e.reason.replaceAll('_', ' ')}'),
            subtitle: Text([formatDateTime(e.createdAt), if (where.isNotEmpty) where].join('\n')),
            isThreeLine: where.isNotEmpty,
          );
        },
      ),
    );
  }
}

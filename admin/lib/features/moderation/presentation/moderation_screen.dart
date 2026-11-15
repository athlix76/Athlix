import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_error.dart';
import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/cursor_list.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../application/moderation_providers.dart';
import '../data/moderation_api.dart';
import 'conversation_messages_screen.dart';

/// Moderation action history (manual and automatic), a form to act on any
/// target, and the entry point for reading a reported conversation.
class ModerationScreen extends ConsumerStatefulWidget {
  const ModerationScreen({super.key});

  @override
  ConsumerState<ModerationScreen> createState() => _ModerationScreenState();
}

class _ModerationScreenState extends ConsumerState<ModerationScreen> {
  final _listKey = GlobalKey<CursorListState<ModerationAction>>();
  final _targetId = TextEditingController();
  final _conversationId = TextEditingController();
  String? _targetType;

  @override
  void dispose() {
    _targetId.dispose();
    _conversationId.dispose();
    super.dispose();
  }

  void _reload() => _listKey.currentState?.reload();

  Future<void> _newAction() async {
    final api = ref.read(moderationApiProvider);
    var targetType = moderationTargetTypes.first;
    var action = moderationActions.first;
    // Not disposed: the dialog's exit animation may still use it (see editor_dialog.dart).
    final id = TextEditingController();
    final r = await confirmAction<ModerationAction>(
      context,
      title: 'Moderation action',
      message: 'Applies immediately and is recorded with your name. The owner is notified for warn and suspend.',
      confirmLabel: 'Apply',
      destructive: true,
      details: StatefulBuilder(
        builder: (ctx, setState) => formColumn([
          FormDropdown<String>(
            label: 'Target type',
            value: targetType,
            items: [for (final t in moderationTargetTypes) (t, t)],
            onChanged: (v) => setState(() => targetType = v ?? targetType),
          ),
          FormText(label: 'Target id', controller: id, keyboardType: TextInputType.number),
          FormDropdown<String>(
            label: 'Action',
            value: action,
            items: [for (final a in moderationActions) (a, a.replaceAll('_', ' '))],
            onChanged: (v) => setState(() => action = v ?? action),
          ),
        ]),
      ),
      reason: const ReasonSpec(),
      run: (reason) {
        final targetId = int.tryParse(id.text.trim());
        if (targetId == null || targetId <= 0) throw ApiError.invalidInput({'targetId': 'Enter a numeric id'});
        return api.act(targetType: targetType, targetId: targetId, action: action, reason: reason!);
      },
      successMessage: 'Action applied',
    );
    if (r != null) _reload();
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(moderationApiProvider);
    final session = ref.watch(sessionProvider);
    final canRead = session.can(Perm.reportsRead);
    final canAct = session.can(Perm.reportsResolve);
    final canReadMessages = session.can(Perm.moderationReadMessages);
    return PageScaffold(
      title: 'Moderation',
      actions: [
        if (canAct) FilledButton.icon(onPressed: _newAction, icon: const Icon(Icons.gavel), label: const Text('New action')),
      ],
      filters: Wrap(spacing: 12, runSpacing: 12, crossAxisAlignment: WrapCrossAlignment.center, children: [
        if (canRead) ...[
          FilterDropdown(
            label: 'Target type',
            value: _targetType,
            options: moderationTargetTypes,
            onChanged: (v) {
              setState(() => _targetType = v);
              _reload();
            },
          ),
          FilterTextField(label: 'Target id', controller: _targetId, width: 160, onSubmitted: (_) => _reload()),
        ],
        if (canReadMessages) ...[
          FilterTextField(
            label: 'Reported conversation id',
            controller: _conversationId,
            width: 220,
            onSubmitted: (v) {
              final id = int.tryParse(v.trim());
              if (id != null) openReportedConversation(context, id);
            },
          ),
        ],
      ]),
      body: !canRead
          ? const Center(child: Text('Enter a reported conversation id above to read its messages.'))
          : CursorList<ModerationAction>(
              key: _listKey,
              load: (cursor) => api.actions(targetType: _targetType, targetId: int.tryParse(_targetId.text.trim()), cursor: cursor),
              emptyText: 'No moderation actions.',
              itemBuilder: (context, a) => ListTile(
                title: Text('${a.action.replaceAll('_', ' ')}  ·  ${a.targetType} #${a.targetId}'),
                subtitle: Text([
                  a.admin?.name ?? 'automatic',
                  formatDateTime(a.createdAt),
                  if (a.reportId != null) 'report #${a.reportId}',
                  if (a.reason != null) a.reason!,
                ].join('  ·  ')),
              ),
            ),
    );
  }
}

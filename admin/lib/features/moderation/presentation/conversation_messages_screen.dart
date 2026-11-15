import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/cursor_list.dart';
import '../../auth/application/session_controller.dart';
import '../application/moderation_providers.dart';
import '../data/moderation_api.dart';

/// Asks for the reason, then opens the reported conversation. The reason is
/// passed in memory (route `extra`), never in the browser URL.
Future<void> openReportedConversation(BuildContext context, int conversationId) async {
  final reason = await askReadReason(context, conversationId);
  if (reason != null && context.mounted) context.go('/moderation/conversations/$conversationId', extra: reason);
}

/// Reason prompt for reading private messages. Returns null when cancelled.
Future<String?> askReadReason(BuildContext context, int conversationId) => confirmAction<String>(
      context,
      title: 'Read messages of conversation #$conversationId?',
      message: 'Private messages may only be read to handle an open report. Your name, the time and '
          'this reason are written to the audit log.',
      confirmLabel: 'Read messages',
      reason: const ReasonSpec(hint: 'For example: report #12, harassment claim'),
      run: (reason) async => reason!,
    );

class ConversationMessagesScreen extends ConsumerStatefulWidget {
  const ConversationMessagesScreen({super.key, required this.conversationId, this.reason});

  final int conversationId;

  /// Null when the page was reloaded or opened by URL; the screen asks again.
  final String? reason;

  @override
  ConsumerState<ConversationMessagesScreen> createState() => _ConversationMessagesScreenState();
}

class _ConversationMessagesScreenState extends ConsumerState<ConversationMessagesScreen> {
  final _listKey = GlobalKey<CursorListState<AdminMessage>>();
  String? _reason;

  @override
  void initState() {
    super.initState();
    _reason = widget.reason;
  }

  Future<void> _ask() async {
    final r = await askReadReason(context, widget.conversationId);
    if (r != null && mounted) setState(() => _reason = r);
  }

  Future<void> _act(AdminMessage m, String action) async {
    final api = ref.read(moderationApiProvider);
    final res = await confirmAction<ModerationAction>(
      context,
      title: '${action == 'hide' ? 'Hide' : action == 'unhide' ? 'Unhide' : 'Remove'} message #${m.id}?',
      message: action == 'unhide' ? 'The message becomes visible again.' : 'Members no longer see the message.',
      confirmLabel: action,
      destructive: action != 'unhide',
      reason: const ReasonSpec(),
      run: (reason) => api.act(targetType: 'message', targetId: m.id, action: action, reason: reason!),
      successMessage: 'Done',
    );
    if (res != null) _listKey.currentState?.reload();
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(moderationApiProvider);
    final reason = _reason;
    final canAct = ref.watch(sessionProvider).can(Perm.reportsResolve);
    return PageScaffold(
      title: 'Conversation #${widget.conversationId}',
      subtitle: reason == null ? null : 'Reason (audited): $reason',
      actions: [TextButton.icon(onPressed: () => context.go('/moderation'), icon: const Icon(Icons.arrow_back), label: const Text('Moderation'))],
      body: reason == null
          ? Center(
              child: FilledButton.icon(
                onPressed: _ask,
                icon: const Icon(Icons.lock_open_outlined),
                label: const Text('Give a reason to read the messages'),
              ),
            )
          : CursorList<AdminMessage>(
              key: _listKey,
              load: (before) => api.messages(widget.conversationId, reason: reason, before: before),
              emptyText: 'No messages.',
              itemBuilder: (context, m) => ListTile(
                title: Text('${m.sender?.label ?? 'system'}  ·  ${formatDateTime(m.createdAt)}'),
                subtitle: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  if (m.replyPreview != null) Text('Reply to ${m.replyPreview}', style: Theme.of(context).textTheme.bodySmall),
                  if (m.body != null) SelectableText(m.body!),
                  if (m.imageUrl != null)
                    TextButton.icon(
                      onPressed: () => openUrl(m.imageUrl!),
                      icon: const Icon(Icons.image_outlined, size: 18),
                      label: const Text('Open image (link expires)'),
                    ),
                ]),
                trailing: Wrap(spacing: 4, crossAxisAlignment: WrapCrossAlignment.center, children: [
                  StatusChip(m.status),
                  if (canAct)
                    PopupMenuButton<String>(
                      tooltip: 'Moderate',
                      onSelected: (a) => _act(m, a),
                      itemBuilder: (_) => [
                        if (m.status == 'visible') const PopupMenuItem(value: 'hide', child: Text('Hide')),
                        if (m.status == 'hidden') const PopupMenuItem(value: 'unhide', child: Text('Unhide')),
                        if (m.status != 'deleted') const PopupMenuItem(value: 'remove', child: Text('Remove')),
                      ],
                    ),
                ]),
              ),
            ),
    );
  }
}

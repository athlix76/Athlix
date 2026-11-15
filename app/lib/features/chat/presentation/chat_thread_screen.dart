import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/format/dates.dart';
import '../../../core/media/pick_image.dart';
import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/user_avatar.dart';
import '../../reports/presentation/report_sheet.dart';
import '../application/chat_socket_provider.dart';
import '../application/thread_controller.dart';
import '../data/chat_models.dart';

/// One conversation (direct, community or event group).
class ChatThreadScreen extends ConsumerWidget {
  const ChatThreadScreen({super.key, required this.conversationId});

  final int conversationId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final conv = ref.watch(conversationProvider(conversationId));
    return AsyncValueView<Conversation>(
      value: conv,
      onRetry: () => ref.invalidate(conversationProvider(conversationId)),
      data: (c) => _ThreadView(conversation: c),
    );
  }
}

class _ThreadView extends ConsumerStatefulWidget {
  const _ThreadView({required this.conversation});

  final Conversation conversation;

  @override
  ConsumerState<_ThreadView> createState() => _ThreadViewState();
}

class _ThreadViewState extends ConsumerState<_ThreadView> {
  final _scroll = ScrollController();
  final _text = TextEditingController();
  Message? _replyTo;

  Conversation get _c => widget.conversation;
  ThreadController get _ctl => ref.read(threadControllerProvider(_c.id).notifier);

  @override
  void initState() {
    super.initState();
    _scroll.addListener(_onScroll);
  }

  @override
  void dispose() {
    _scroll.dispose();
    _text.dispose();
    super.dispose();
  }

  /// The list is reversed: the top (oldest) is at maxScrollExtent.
  void _onScroll() {
    if (_scroll.position.pixels >= _scroll.position.maxScrollExtent - 400) unawaited(_ctl.loadMore());
  }

  void _send() {
    final text = _text.text;
    if (text.trim().isEmpty) return;
    _ctl.sendText(text, replyTo: _replyTo);
    _text.clear();
    setState(() => _replyTo = null);
  }

  Future<void> _sendImage() async {
    final file = await pickImageFile();
    if (file == null || !mounted) return;
    _ctl.sendImage(file, replyTo: _replyTo);
    setState(() => _replyTo = null);
  }

  Future<void> _messageActions(Message m) async {
    final me = ref.read(currentUserIdProvider);
    final mine = m.sender?.id == me;
    final canDelete = m.isSent && !m.isDeleted && (mine || _c.canModerate);
    final canReport = m.isSent && !mine && !m.isSystem && !m.isDeleted;
    final canReply = m.isSent && !m.isDeleted && !m.isSystem && _c.isActive;
    final canCopy = !m.isDeleted && (m.body?.isNotEmpty ?? false);
    final action = await showModalBottomSheet<String>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (canReply)
              ListTile(leading: const Icon(Icons.reply), title: const Text('Reply'), onTap: () => Navigator.pop(ctx, 'reply')),
            if (canCopy)
              ListTile(leading: const Icon(Icons.copy), title: const Text('Copy'), onTap: () => Navigator.pop(ctx, 'copy')),
            if (canDelete)
              ListTile(
                leading: const Icon(Icons.delete_outline),
                title: const Text('Delete'),
                onTap: () => Navigator.pop(ctx, 'delete'),
              ),
            if (canReport)
              ListTile(leading: const Icon(Icons.flag_outlined), title: const Text('Report'), onTap: () => Navigator.pop(ctx, 'report')),
          ],
        ),
      ),
    );
    if (!mounted || action == null) return;
    switch (action) {
      case 'reply':
        setState(() => _replyTo = m);
      case 'copy':
        await Clipboard.setData(ClipboardData(text: m.body ?? ''));
        if (mounted) showInfoSnack(context, 'Copied');
      case 'delete':
        final ok = await confirm(context, title: 'Delete message?', body: 'It will be removed for everyone.', ok: 'Delete');
        if (!ok) return;
        try {
          await _ctl.delete(m);
        } catch (e) {
          if (mounted) showErrorSnack(context, e);
        }
      case 'report':
        await showReportSheet(context, targetType: ReportTarget.message, targetId: m.id);
    }
  }

  Future<void> _failedActions(Message m) async {
    final cid = m.clientMsgId;
    if (cid == null) return;
    final action = await showModalBottomSheet<String>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(leading: const Icon(Icons.refresh), title: const Text('Try again'), onTap: () => Navigator.pop(ctx, 'retry')),
            ListTile(leading: const Icon(Icons.close), title: const Text('Discard'), onTap: () => Navigator.pop(ctx, 'discard')),
          ],
        ),
      ),
    );
    if (action == 'retry') _ctl.retry(cid);
    if (action == 'discard') _ctl.discard(cid);
  }

  Future<void> _menu(String value) async {
    final convCtl = ref.read(conversationControllerProvider);
    try {
      switch (value) {
        case 'members':
          await context.push(Routes.conversationMembers(_c.id));
        case 'community':
          await context.push(Routes.community(_c.id));
        case 'event':
          if (_c.eventId != null) await context.push(Routes.event(_c.eventId!));
        case 'profile':
          if (_c.otherUser != null) await context.push(Routes.profile(_c.otherUser!.id));
        case 'unmute':
          await convCtl.mute(_c.id, null);
        case 'mute':
          final until = await _pickMute();
          if (until != null) await convCtl.mute(_c.id, until);
        case 'leave':
          final ok = await confirm(context, title: 'Leave this chat?', ok: 'Leave');
          if (!ok) return;
          await convCtl.leave(_c.id);
          if (mounted) context.pop();
        case 'report':
          await showReportSheet(context, targetType: ReportTarget.conversation, targetId: _c.id);
      }
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  Future<DateTime?> _pickMute() => showModalBottomSheet<DateTime>(
        context: context,
        builder: (ctx) => SafeArea(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              ListTile(
                title: const Text('For 8 hours'),
                onTap: () => Navigator.pop(ctx, DateTime.now().add(const Duration(hours: 8))),
              ),
              ListTile(
                title: const Text('For 1 week'),
                onTap: () => Navigator.pop(ctx, DateTime.now().add(const Duration(days: 7))),
              ),
              ListTile(
                title: const Text('Until I turn it back on'),
                onTap: () => Navigator.pop(ctx, ConversationController.forever),
              ),
            ],
          ),
        ),
      );

  @override
  Widget build(BuildContext context) {
    final provider = threadControllerProvider(_c.id);
    final s = ref.watch(provider);
    ref.listen(provider.select((x) => x.notice), (_, n) {
      if (n != null) showInfoSnack(context, n.text);
    });
    final me = ref.watch(currentUserIdProvider);
    final config = ref.watch(appConfigProvider);
    return Scaffold(
      appBar: AppBar(
        titleSpacing: 0,
        title: InkWell(
          onTap: _c.isDirect ? () => _menu('profile') : (_c.type == ConversationType.community ? () => _menu('community') : null),
          child: Row(
            children: [
              UserAvatar(name: _c.displayTitle, url: _c.displayAvatar, radius: 16),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(_c.displayTitle, overflow: TextOverflow.ellipsis),
                    Text(
                      _subtitle(s),
                      style: Theme.of(context).textTheme.labelSmall,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
        actions: [
          PopupMenuButton<String>(
            onSelected: _menu,
            itemBuilder: (_) => [
              if (_c.isDirect && _c.otherUser != null) const PopupMenuItem(value: 'profile', child: Text('View profile')),
              if (_c.type == ConversationType.community)
                const PopupMenuItem(value: 'community', child: Text('Community info')),
              if (_c.type == ConversationType.event && _c.eventId != null)
                const PopupMenuItem(value: 'event', child: Text('Open event')),
              if (!_c.isDirect) const PopupMenuItem(value: 'members', child: Text('Members')),
              if (_c.muted)
                const PopupMenuItem(value: 'unmute', child: Text('Unmute'))
              else
                const PopupMenuItem(value: 'mute', child: Text('Mute')),
              if (!_c.isDirect && _c.isActive) const PopupMenuItem(value: 'leave', child: Text('Leave')),
              const PopupMenuItem(value: 'report', child: Text('Report')),
            ],
          ),
        ],
      ),
      body: Column(
        children: [
          Expanded(child: _messages(s, me)),
          if (s.typing.isNotEmpty) _typingLine(s),
          if (_c.isActive)
            _Composer(
              controller: _text,
              replyTo: _replyTo,
              maxLength: config.chatMessageMaxLength,
              canSendImage: config.chatMediaEnabled,
              onCancelReply: () => setState(() => _replyTo = null),
              onChanged: (_) => _ctl.userTyping(),
              onSend: _send,
              onImage: _sendImage,
            )
          else
            SafeArea(
              top: false,
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Text(
                  'This conversation is ${_c.status}. You can read it but not send messages.',
                  textAlign: TextAlign.center,
                ),
              ),
            ),
        ],
      ),
    );
  }

  String _subtitle(ThreadState s) {
    if (s.typing.isNotEmpty) return 'typing…';
    if (_c.isDirect) return _c.otherUser?.subtitle ?? '';
    return '${_c.memberCount} members';
  }

  Widget _messages(ThreadState s, int? me) {
    if (s.loading && s.messages.isEmpty) return const Center(child: CircularProgressIndicator());
    if (s.error != null && s.messages.isEmpty) return ErrorView(error: s.error, onRetry: _ctl.reload);
    if (s.messages.isEmpty) return const EmptyView(text: 'No messages yet. Say hello!', icon: Icons.chat_bubble_outline);
    final seenLabel = _seenLabel(s, me);
    final lastMineId = s.messages.where((m) => m.isSent && m.sender?.id == me).firstOrNull?.id;
    final count = s.messages.length + (s.loadingMore ? 1 : 0);
    return ListView.builder(
      controller: _scroll,
      reverse: true,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      itemCount: count,
      itemBuilder: (context, i) {
        if (i >= s.messages.length) {
          return const Padding(padding: EdgeInsets.all(12), child: Center(child: CircularProgressIndicator()));
        }
        final m = s.messages[i];
        final older = i + 1 < s.messages.length ? s.messages[i + 1] : null;
        final mine = !m.isSent ? !m.isSystem : m.sender?.id == me;
        final cid = m.clientMsgId;
        final showSender = !_c.isDirect && !mine && !m.isSystem && older?.sender?.id != m.sender?.id;
        return _Bubble(
          message: m,
          mine: mine,
          showSender: showSender,
          showDayDivider: _newDay(m, older),
          pending: cid != null && s.pending.contains(cid),
          failed: cid != null && s.failed.contains(cid),
          footer: m.id == lastMineId ? seenLabel : null,
          onLongPress: () => _messageActions(m),
          onTapFailed: () => _failedActions(m),
        );
      },
    );
  }

  static bool _newDay(Message m, Message? older) {
    final a = m.createdAt?.toLocal();
    final b = older?.createdAt?.toLocal();
    if (a == null) return false;
    if (b == null) return true;
    return a.year != b.year || a.month != b.month || a.day != b.day;
  }

  /// Receipts seen this session (the contract has no initial read state).
  String? _seenLabel(ThreadState s, int? me) {
    final lastMine = s.messages.where((m) => m.isSent && m.sender?.id == me).firstOrNull;
    if (lastMine == null) return null;
    final readers = s.readUpTo.entries.where((e) => e.key != me && e.value >= lastMine.id).length;
    if (readers == 0) return null;
    return _c.isDirect ? 'Seen' : 'Seen by $readers';
  }

  Widget _typingLine(ThreadState s) {
    final names = s.typing.map((uid) {
      if (_c.isDirect && _c.otherUser?.id == uid) return _c.otherUser!.displayName;
      return s.messages.where((m) => m.sender?.id == uid).firstOrNull?.sender?.displayName ?? 'Someone';
    }).toList();
    final text = names.length == 1 ? '${names.first} is typing…' : '${names.length} people are typing…';
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 4),
      child: Align(
        alignment: Alignment.centerLeft,
        child: Text(text, style: Theme.of(context).textTheme.labelSmall),
      ),
    );
  }
}

class _Bubble extends StatelessWidget {
  const _Bubble({
    required this.message,
    required this.mine,
    required this.showSender,
    required this.showDayDivider,
    required this.pending,
    required this.failed,
    this.footer,
    required this.onLongPress,
    required this.onTapFailed,
  });

  final Message message;
  final bool mine;
  final bool showSender;
  final bool showDayDivider;
  final bool pending;
  final bool failed;
  final String? footer;
  final VoidCallback onLongPress;
  final VoidCallback onTapFailed;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    final m = message;
    final divider = showDayDivider
        ? Padding(
            padding: const EdgeInsets.symmetric(vertical: 8),
            child: Center(child: Text(formatDate(m.createdAt), style: theme.textTheme.labelSmall)),
          )
        : null;

    if (m.isSystem) {
      return Column(
        children: [
          if (divider != null) divider,
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 6),
            child: Center(
              child: Text(m.body ?? '', style: theme.textTheme.labelMedium, textAlign: TextAlign.center),
            ),
          ),
        ],
      );
    }

    final bg = mine ? scheme.primaryContainer : scheme.surfaceContainerHighest;
    final fg = mine ? scheme.onPrimaryContainer : scheme.onSurface;
    final content = <Widget>[
      if (showSender && m.sender != null)
        Padding(
          padding: const EdgeInsets.only(bottom: 2),
          child: Text(m.sender!.displayName, style: theme.textTheme.labelMedium?.copyWith(color: scheme.primary)),
        ),
      if (m.replyTo != null && !m.isDeleted) _ReplyQuote(reply: m.replyTo!, color: fg),
      if (m.isDeleted)
        Text('Message deleted', style: TextStyle(color: fg, fontStyle: FontStyle.italic))
      else ...[
        if (m.kind == MessageKind.image)
          ClipRRect(
            borderRadius: BorderRadius.circular(8),
            child: SizedBox(
              width: 220,
              child: m.imageUrl == null
                  ? const SizedBox(height: 160, child: Center(child: Icon(Icons.image_outlined)))
                  : SignedImage(url: m.imageUrl!, height: 220),
            ),
          ),
        if (m.body != null && m.body!.isNotEmpty) Text(m.body!, style: TextStyle(color: fg)),
      ],
      const SizedBox(height: 2),
      Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(formatTime(m.createdAt), style: theme.textTheme.labelSmall?.copyWith(color: fg.withAlpha(179))),
          if (mine) ...[
            const SizedBox(width: 4),
            Icon(
              failed ? Icons.error_outline : (pending ? Icons.schedule : Icons.check),
              size: 14,
              color: failed ? scheme.error : fg.withAlpha(179),
            ),
          ],
        ],
      ),
    ];

    return Column(
      crossAxisAlignment: mine ? CrossAxisAlignment.end : CrossAxisAlignment.start,
      children: [
        if (divider != null) divider,
        GestureDetector(
          onLongPress: onLongPress,
          onTap: failed ? onTapFailed : null,
          child: Container(
            margin: const EdgeInsets.symmetric(vertical: 2),
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
            constraints: BoxConstraints(maxWidth: MediaQuery.sizeOf(context).width * 0.75),
            decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(14)),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: content,
            ),
          ),
        ),
        if (failed)
          Text('Not sent. Tap to retry.', style: theme.textTheme.labelSmall?.copyWith(color: scheme.error)),
        if (footer != null) Text(footer!, style: theme.textTheme.labelSmall),
      ],
    );
  }
}

class _ReplyQuote extends StatelessWidget {
  const _ReplyQuote({required this.reply, required this.color});

  final ReplyPreview reply;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(bottom: 4),
      padding: const EdgeInsets.only(left: 8),
      decoration: BoxDecoration(border: Border(left: BorderSide(color: color.withAlpha(128), width: 3))),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (reply.senderName != null)
            Text(reply.senderName!, style: Theme.of(context).textTheme.labelSmall?.copyWith(color: color)),
          Text(
            reply.bodyPreview ?? '',
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(color: color.withAlpha(204), fontSize: 13),
          ),
        ],
      ),
    );
  }
}

class _Composer extends StatelessWidget {
  const _Composer({
    required this.controller,
    required this.replyTo,
    required this.maxLength,
    required this.canSendImage,
    required this.onCancelReply,
    required this.onChanged,
    required this.onSend,
    required this.onImage,
  });

  final TextEditingController controller;
  final Message? replyTo;
  final int? maxLength;
  final bool canSendImage;
  final VoidCallback onCancelReply;
  final ValueChanged<String> onChanged;
  final VoidCallback onSend;
  final VoidCallback onImage;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final r = replyTo;
    return SafeArea(
      top: false,
      child: Material(
        color: theme.colorScheme.surface,
        elevation: 2,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (r != null)
              ListTile(
                dense: true,
                leading: const Icon(Icons.reply),
                title: Text('Replying to ${r.sender?.displayName ?? 'message'}'),
                subtitle: Text(r.preview, maxLines: 1, overflow: TextOverflow.ellipsis),
                trailing: IconButton(icon: const Icon(Icons.close), onPressed: onCancelReply),
              ),
            Padding(
              padding: const EdgeInsets.fromLTRB(8, 6, 8, 6),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  if (canSendImage)
                    IconButton(tooltip: 'Send photo', icon: const Icon(Icons.photo_outlined), onPressed: onImage),
                  Expanded(
                    child: TextField(
                      controller: controller,
                      minLines: 1,
                      maxLines: 5,
                      maxLength: maxLength,
                      textCapitalization: TextCapitalization.sentences,
                      onChanged: onChanged,
                      decoration: const InputDecoration(
                        hintText: 'Message',
                        counterText: '',
                        isDense: true,
                      ),
                    ),
                  ),
                  const SizedBox(width: 4),
                  IconButton.filled(tooltip: 'Send', icon: const Icon(Icons.send), onPressed: onSend),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

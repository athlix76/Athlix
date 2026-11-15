import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/user_avatar.dart';
import '../../../core/ws/chat_socket.dart';
import '../application/chat_socket_provider.dart';
import '../data/chat_api.dart';
import '../data/chat_models.dart';

/// Chat tab: every conversation, latest activity first.
class ConversationsScreen extends ConsumerStatefulWidget {
  const ConversationsScreen({super.key});

  @override
  ConsumerState<ConversationsScreen> createState() => _ConversationsScreenState();
}

class _ConversationsScreenState extends ConsumerState<ConversationsScreen> with WidgetsBindingObserver {
  final _list = CursorListController<Conversation>();
  StreamSubscription<ServerFrame>? _frames;
  Timer? _debounce;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _frames = ref.read(chatSocketProvider).frames.listen(_onFrame);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _frames?.cancel();
    _debounce?.cancel();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      unawaited(ref.read(chatSocketProvider).ensureConnected());
      _scheduleRefresh();
    }
  }

  void _onFrame(ServerFrame f) {
    if (f.type == WsType.messageNew || f.type == WsType.messageDeleted) _scheduleRefresh();
  }

  /// Many frames in a burst → one reload (order and unread counts come from the server).
  void _scheduleRefresh() {
    _debounce?.cancel();
    _debounce = Timer(const Duration(seconds: 1), () {
      if (mounted) unawaited(_list.refresh());
    });
  }

  Future<void> _open(Conversation c) async {
    await context.push(Routes.conversation(c.id));
    if (mounted) unawaited(_list.refresh());
  }

  @override
  Widget build(BuildContext context) {
    // Re-subscribe when the socket is rebuilt (user changed).
    ref.listen(chatSocketProvider, (_, next) {
      _frames?.cancel();
      _frames = next.frames.listen(_onFrame);
    });
    final status = ref.watch(chatSocketStatusProvider).valueOrNull;
    return Scaffold(
      appBar: AppBar(
        title: const Text('Chat'),
        bottom: status == null || status == SocketStatus.connected
            ? null
            : const PreferredSize(preferredSize: Size.fromHeight(2), child: LinearProgressIndicator(minHeight: 2)),
        actions: [
          IconButton(
            tooltip: 'Communities',
            icon: const Icon(Icons.groups_outlined),
            onPressed: () => context.push(Routes.communities),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _list.refresh,
        child: CursorListView<Conversation>(
          controller: _list,
          fetch: (cursor) => ref.read(chatApiProvider).conversations(cursor: cursor),
          emptyText: 'No conversations yet. Message someone from their profile, or join a community.',
          separated: true,
          itemBuilder: (context, c, _) => ConversationTile(conversation: c, onTap: () => _open(c)),
        ),
      ),
    );
  }
}

class ConversationTile extends StatelessWidget {
  const ConversationTile({super.key, required this.conversation, this.onTap});

  final Conversation conversation;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final c = conversation;
    final scheme = Theme.of(context).colorScheme;
    final last = c.lastMessage;
    final unread = c.unreadCount > 0;
    final preview = last == null ? _typeLabel(c) : _lastLine(last);
    return ListTile(
      onTap: onTap,
      leading: UserAvatar(name: c.displayTitle, url: c.displayAvatar),
      title: Row(
        children: [
          Expanded(
            child: Text(
              c.displayTitle,
              overflow: TextOverflow.ellipsis,
              style: unread ? const TextStyle(fontWeight: FontWeight.w600) : null,
            ),
          ),
          if (c.muted) Icon(Icons.notifications_off_outlined, size: 16, color: scheme.outline),
        ],
      ),
      subtitle: Text(preview, maxLines: 1, overflow: TextOverflow.ellipsis),
      trailing: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          Text(timeAgo(last?.createdAt ?? c.updatedAt), style: Theme.of(context).textTheme.labelSmall),
          if (unread) ...[
            const SizedBox(height: 4),
            Badge(label: Text(c.unreadCount > 99 ? '99+' : '${c.unreadCount}')),
          ],
        ],
      ),
    );
  }

  static String _typeLabel(Conversation c) => switch (c.type) {
        ConversationType.community => 'Community · ${c.memberCount} members',
        ConversationType.event => 'Event group · ${c.memberCount} members',
        _ => 'Say hello',
      };

  String _lastLine(Message m) {
    if (conversation.isDirect || m.sender == null) return m.preview;
    return '${m.sender!.displayName}: ${m.preview}';
  }
}

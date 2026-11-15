import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/env.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/ws/chat_socket.dart';
import '../data/chat_api.dart';
import '../data/chat_models.dart';

/// One WebSocket for the signed-in user. Rebuilt (old socket closed) when the
/// user changes; not started while signed out. Tickets come from
/// `POST /chat/ws-ticket` and are never logged.
final chatSocketProvider = Provider<ChatSocket>((ref) {
  final userId = ref.watch(currentUserIdProvider);
  final api = ref.watch(chatApiProvider);
  final socket = ChatSocket(fetchTicket: api.wsTicket, apiBase: Uri.parse(Env.apiBaseUrl));
  ref.onDispose(() => unawaited(socket.dispose()));
  if (userId != null) unawaited(socket.start());
  return socket;
});

/// Live socket status, for "connecting…" hints and poll fallback.
final chatSocketStatusProvider = StreamProvider.autoDispose<SocketStatus>((ref) async* {
  final socket = ref.watch(chatSocketProvider);
  yield socket.status;
  yield* socket.statusChanges;
});

final conversationProvider = FutureProvider.autoDispose.family<Conversation, int>(
  (ref, id) => ref.watch(chatApiProvider).conversation(id),
);

/// Conversation-level writes. Each one refreshes the cached conversation.
class ConversationController {
  ConversationController(this._ref);

  final Ref _ref;

  ChatApi get _api => _ref.read(chatApiProvider);

  /// "Until I unmute": the contract has no "forever" value, so use a far date.
  static final DateTime forever = DateTime.utc(9999, 12, 31);

  /// [until] null unmutes.
  Future<void> mute(int id, DateTime? until) async {
    await _api.mute(id, until);
    _ref.invalidate(conversationProvider(id));
  }

  Future<void> leave(int id) async {
    await _api.leave(id);
    _ref.invalidate(conversationProvider(id));
  }
}

final conversationControllerProvider = Provider<ConversationController>(ConversationController.new);

/// Chat tab badge (`GET /chat/unread-count`). HomeShell refreshes it on new
/// or deleted messages; the thread screen invalidates it after marking read.
final unreadChatProvider = FutureProvider.autoDispose<ChatUnread>(
  (ref) => ref.watch(chatApiProvider).unreadCount(),
);

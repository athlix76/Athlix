import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/envelope.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import '../../../core/ws/chat_socket.dart';
import 'chat_models.dart';

/// REST side of chat (docs/api.md "Chat"). The WebSocket lives in
/// `application/chat_socket_provider.dart`; every action here also works
/// without it.
class ChatApi {
  const ChatApi(this._api);

  final ApiClient _api;

  /// Single-use ticket for the WebSocket. Never log it.
  Future<WsTicket> wsTicket() => _api.post('/chat/ws-ticket', (d) => WsTicket.fromJson(asMap(d)));

  Future<Paged<Conversation>> conversations({String? cursor}) =>
      _api.page('/chat/conversations', Conversation.fromJson, query: {'cursor': cursor});

  /// Get or create a direct conversation (403 `FORBIDDEN` when DM rules refuse).
  Future<Conversation> direct(int userId) =>
      _api.post('/chat/direct', (d) => Conversation.fromJson(asMap(d)), body: {'userId': userId});

  Future<Conversation> conversation(int id) =>
      _api.get('/chat/conversations/$id', (d) => Conversation.fromJson(asMap(d)));

  /// Newest first. [before] is the oldest message id already shown.
  Future<Paged<Message>> messages(int conversationId, {int? before, int? limit}) => _api.page(
        '/chat/conversations/$conversationId/messages',
        Message.fromJson,
        query: {'before': before, 'limit': limit},
      );

  /// Idempotent on [clientMsgId]: retrying returns the same message.
  Future<Message> send(
    int conversationId, {
    required String clientMsgId,
    required String kind,
    String? body,
    int? mediaUploadId,
    int? replyToId,
  }) =>
      _api.post(
        '/chat/conversations/$conversationId/messages',
        (d) => Message.fromJson(asMap(d)),
        body: {
          'clientMsgId': clientMsgId,
          'kind': kind,
          if (body != null) 'body': body,
          if (mediaUploadId != null) 'mediaUploadId': mediaUploadId,
          if (replyToId != null) 'replyToId': replyToId,
        },
      );

  Future<void> deleteMessage(int messageId) => _api.delete('/chat/messages/$messageId');

  /// `GET /chat/unread-count` → messages unread across unmuted conversations.
  Future<ChatUnread> unreadCount() => _api.get('/chat/unread-count', (d) => ChatUnread.fromJson(asMap(d)));

  Future<void> markRead(int conversationId, int messageId) =>
      _api.postVoid('/chat/conversations/$conversationId/read', body: {'messageId': messageId});

  /// [mutedUntil] null unmutes.
  Future<void> mute(int conversationId, DateTime? mutedUntil) => _api.postVoid(
        '/chat/conversations/$conversationId/mute',
        body: {'mutedUntil': formatRfc3339OrNull(mutedUntil)},
      );

  Future<void> leave(int conversationId) => _api.postVoid('/chat/conversations/$conversationId/leave');

  Future<Paged<ConversationMember>> members(int conversationId, {String? cursor}) => _api.page(
        '/chat/conversations/$conversationId/members',
        ConversationMember.fromJson,
        query: {'cursor': cursor},
      );

  /// Event group chat (organiser + registered players; 404 otherwise).
  Future<Conversation> eventGroup(int eventId) =>
      _api.get('/events/$eventId/group', (d) => Conversation.fromJson(asMap(d)));
}

final chatApiProvider = Provider<ChatApi>((ref) => ChatApi(ref.watch(apiClientProvider)));

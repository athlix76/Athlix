import '../../../core/json/json_utils.dart';
import '../../../core/models/user_summary.dart';

/// Wire values from docs/api.md "Chat".
abstract final class ConversationType {
  static const direct = 'direct';
  static const community = 'community';
  static const event = 'event';
}

abstract final class MemberRole {
  static const owner = 'owner';
  static const admin = 'admin';
  static const member = 'member';

  static bool canModerate(String? role) => role == owner || role == admin;
}

abstract final class MessageKind {
  static const text = 'text';
  static const image = 'image';
  static const system = 'system';
}

abstract final class MessageStatus {
  static const visible = 'visible';
  static const deleted = 'deleted';
  static const hidden = 'hidden';
}

/// `Message.replyTo`.
class ReplyPreview {
  const ReplyPreview({required this.id, this.senderName, this.bodyPreview});

  final int id;
  final String? senderName;
  final String? bodyPreview;

  factory ReplyPreview.fromJson(Map<String, dynamic> json) => ReplyPreview(
        id: asInt(json['id']),
        senderName: asStringOrNull(json['senderName']),
        bodyPreview: asStringOrNull(json['bodyPreview']),
      );

  static ReplyPreview? maybe(Object? v) {
    final m = asMapOrNull(v);
    return m == null ? null : ReplyPreview.fromJson(m);
  }

  Map<String, dynamic> toJson() => {'id': id, 'senderName': senderName, 'bodyPreview': bodyPreview};
}

/// `Message`. `id` is 0 for an optimistic message not yet acknowledged;
/// such messages are matched by [clientMsgId].
class Message {
  const Message({
    required this.id,
    required this.conversationId,
    this.sender,
    required this.kind,
    this.body,
    this.imageUrl,
    this.replyTo,
    this.clientMsgId,
    required this.status,
    this.createdAt,
  });

  final int id;
  final int conversationId;

  /// Null for system messages.
  final UserSummary? sender;
  final String kind;
  final String? body;

  /// Short-lived signed URL; never cached across sessions.
  final String? imageUrl;
  final ReplyPreview? replyTo;
  final String? clientMsgId;
  final String status;
  final DateTime? createdAt;

  bool get isSystem => kind == MessageKind.system;
  bool get isDeleted => status != MessageStatus.visible;
  bool get isSent => id > 0;

  factory Message.fromJson(Map<String, dynamic> json) => Message(
        id: asInt(json['id']),
        conversationId: asInt(json['conversationId']),
        sender: UserSummary.maybe(json['sender']),
        kind: asString(json['kind'], MessageKind.text),
        body: asStringOrNull(json['body']),
        imageUrl: asStringOrNull(json['imageUrl']),
        replyTo: ReplyPreview.maybe(json['replyTo']),
        clientMsgId: asStringOrNull(json['clientMsgId']),
        status: asString(json['status'], MessageStatus.visible),
        createdAt: asDateTimeOrNull(json['createdAt']),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'conversationId': conversationId,
        'sender': sender?.toJson(),
        'kind': kind,
        'body': body,
        'imageUrl': imageUrl,
        'replyTo': replyTo?.toJson(),
        'clientMsgId': clientMsgId,
        'status': status,
        'createdAt': formatRfc3339OrNull(createdAt),
      };

  Message markDeleted() => Message(
        id: id,
        conversationId: conversationId,
        sender: sender,
        kind: kind,
        replyTo: replyTo,
        clientMsgId: clientMsgId,
        status: MessageStatus.deleted,
        createdAt: createdAt,
      );

  /// Short text for conversation lists and reply bars.
  String get preview {
    if (isDeleted) return 'Message deleted';
    if (kind == MessageKind.image) return body == null || body!.isEmpty ? 'Photo' : 'Photo: $body';
    return body ?? '';
  }

  /// Message text is user content: never include it in logs.
  @override
  String toString() => 'Message($id, $conversationId, $kind, $status)';
}

/// `Conversation`.
class Conversation {
  const Conversation({
    required this.id,
    required this.type,
    this.title,
    this.avatarUrl,
    this.otherUser,
    this.eventId,
    required this.memberCount,
    this.myRole,
    required this.muted,
    required this.unreadCount,
    this.lastMessage,
    required this.status,
    this.updatedAt,
  });

  final int id;
  final String type;
  final String? title;
  final String? avatarUrl;

  /// Direct conversations only.
  final UserSummary? otherUser;
  final int? eventId;
  final int memberCount;
  final String? myRole;
  final bool muted;
  final int unreadCount;
  final Message? lastMessage;
  final String status;
  final DateTime? updatedAt;

  bool get isDirect => type == ConversationType.direct;
  bool get isActive => status == 'active';
  bool get canModerate => MemberRole.canModerate(myRole);

  String get displayTitle {
    final t = title;
    if (t != null && t.isNotEmpty) return t;
    return otherUser?.displayName ?? (type == ConversationType.event ? 'Event group' : 'Chat');
  }

  String? get displayAvatar => isDirect ? otherUser?.photoUrl : avatarUrl;

  factory Conversation.fromJson(Map<String, dynamic> json) {
    final last = asMapOrNull(json['lastMessage']);
    return Conversation(
      id: asInt(json['id']),
      type: asString(json['type'], ConversationType.direct),
      title: asStringOrNull(json['title']),
      avatarUrl: asStringOrNull(json['avatarUrl']),
      otherUser: UserSummary.maybe(json['otherUser']),
      eventId: asIntOrNull(json['eventId']),
      memberCount: asInt(json['memberCount']),
      myRole: asStringOrNull(json['myRole']),
      muted: asBool(json['muted']),
      unreadCount: asInt(json['unreadCount']),
      lastMessage: last == null ? null : Message.fromJson(last),
      status: asString(json['status'], 'active'),
      updatedAt: asDateTimeOrNull(json['updatedAt']),
    );
  }

  Map<String, dynamic> toJson() => {
        'id': id,
        'type': type,
        'title': title,
        'avatarUrl': avatarUrl,
        'otherUser': otherUser?.toJson(),
        'eventId': eventId,
        'memberCount': memberCount,
        'myRole': myRole,
        'muted': muted,
        'unreadCount': unreadCount,
        'lastMessage': lastMessage?.toJson(),
        'status': status,
        'updatedAt': formatRfc3339OrNull(updatedAt),
      };

  Conversation copyWith({bool? muted, int? unreadCount, Message? lastMessage, DateTime? updatedAt}) =>
      Conversation(
        id: id,
        type: type,
        title: title,
        avatarUrl: avatarUrl,
        otherUser: otherUser,
        eventId: eventId,
        memberCount: memberCount,
        myRole: myRole,
        muted: muted ?? this.muted,
        unreadCount: unreadCount ?? this.unreadCount,
        lastMessage: lastMessage ?? this.lastMessage,
        status: status,
        updatedAt: updatedAt ?? this.updatedAt,
      );
}

/// Item of `GET /chat/conversations/:id/members`.
class ConversationMember {
  const ConversationMember({required this.user, required this.role, this.joinedAt});

  final UserSummary user;
  final String role;
  final DateTime? joinedAt;

  factory ConversationMember.fromJson(Map<String, dynamic> json) => ConversationMember(
        user: UserSummary.fromJson(asMap(json['user'])),
        role: asString(json['role'], MemberRole.member),
        joinedAt: asDateTimeOrNull(json['joinedAt']),
      );
}

/// `GET /chat/unread-count` (muted conversations are not counted).
class ChatUnread {
  const ChatUnread({required this.conversations, required this.messages});

  final int conversations;
  final int messages;

  factory ChatUnread.fromJson(Map<String, dynamic> json) => ChatUnread(
        conversations: asInt(json['conversations']),
        messages: asInt(json['messages']),
      );
}

import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';
import '../../../core/models/common.dart';

const moderationTargetTypes = ['user', 'event', 'post', 'comment', 'message', 'conversation'];
const moderationActions = ['hide', 'unhide', 'remove', 'suspend_user', 'unsuspend_user', 'warn'];

/// `ModerationAction`. [admin] null = automatic (auto-hide).
class ModerationAction {
  const ModerationAction({
    required this.id,
    this.admin,
    required this.targetType,
    required this.targetId,
    required this.action,
    this.reason,
    this.reportId,
    this.createdAt,
  });

  final int id;
  final AdminRef? admin;
  final String targetType;
  final int targetId;
  final String action;
  final String? reason;
  final int? reportId;
  final DateTime? createdAt;

  factory ModerationAction.fromJson(Json j) => ModerationAction(
        id: asInt(j['id']),
        admin: AdminRef.maybe(j['admin']),
        targetType: asString(j['targetType']),
        targetId: asInt(j['targetId']),
        action: asString(j['action']),
        reason: asStringOrNull(j['reason']),
        reportId: asIntOrNull(j['reportId']),
        createdAt: asDate(j['createdAt']),
      );
}

/// `Message` as an admin sees it in a reported conversation.
class AdminMessage {
  const AdminMessage({required this.id, this.sender, required this.kind, this.body, this.imageUrl, this.replyPreview, required this.status, this.createdAt});

  final int id;
  final UserSummary? sender;
  final String kind;
  final String? body;
  final String? imageUrl;
  final String? replyPreview;
  final String status;
  final DateTime? createdAt;

  factory AdminMessage.fromJson(Json j) {
    final reply = asJsonOrNull(j['replyTo']);
    return AdminMessage(
      id: asInt(j['id']),
      sender: UserSummary.maybe(j['sender']),
      kind: asString(j['kind'], 'text'),
      body: asStringOrNull(j['body']),
      imageUrl: asStringOrNull(j['imageUrl']),
      replyPreview: reply == null ? null : '${asString(reply['senderName'])}: ${asString(reply['bodyPreview'])}',
      status: asString(j['status'], 'visible'),
      createdAt: asDate(j['createdAt']),
    );
  }
}

class ModerationApi {
  ModerationApi(this._client);

  final ApiClient _client;

  /// `GET /admin/moderation/actions` (reports.read).
  Future<PageResult<ModerationAction>> actions({String? targetType, int? targetId, String? cursor}) => _client.page(
        '/admin/moderation/actions',
        ModerationAction.fromJson,
        query: {'targetType': targetType, 'targetId': targetId},
        cursor: cursor,
      );

  /// `POST /admin/moderation/actions` (reports.resolve).
  Future<ModerationAction> act({required String targetType, required int targetId, required String action, required String reason}) async =>
      ModerationAction.fromJson(asJson(await _client.post('/admin/moderation/actions', body: {
        'targetType': targetType,
        'targetId': targetId,
        'action': action,
        'reason': reason,
      })));

  /// `GET /admin/conversations/:id/messages` (moderation.read_messages), newest
  /// first. Only works while the conversation (or one of its messages) has an
  /// open report; the backend audits every read. The admin's reason goes in
  /// `?reason=` because the backend CORS allows no custom request headers.
  /// The returned cursor is the id of the oldest message loaded (`before`).
  Future<PageResult<AdminMessage>> messages(int conversationId, {required String reason, String? before}) async {
    final p = await _client.page(
      '/admin/conversations/$conversationId/messages',
      AdminMessage.fromJson,
      query: {'before': before, 'reason': reason},
      limit: 50,
    );
    return PageResult(p.items, p.hasMore && p.items.isNotEmpty ? '${p.items.last.id}' : null);
  }
}

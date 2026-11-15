import '../../../core/json/json_utils.dart';

/// `Notification` (docs/api.md "Notifications"). Named `AppNotification` so it
/// does not clash with Flutter's `Notification` class.
///
/// `data` holds deep-link ids only (`eventId`, `conversationId`, `userId`,
/// `postId`, `communityId`, `consultationRequestId`). Title and body contain
/// display names only, never PII.
class AppNotification {
  const AppNotification({
    required this.id,
    required this.type,
    required this.title,
    required this.body,
    required this.data,
    required this.readAt,
    required this.createdAt,
  });

  final int id;
  final String type;
  final String title;
  final String body;
  final Map<String, dynamic> data;
  final DateTime? readAt;
  final DateTime? createdAt;

  bool get isRead => readAt != null;

  /// The deep-link payload plus `type`, as [routeForNotificationData] expects.
  Map<String, dynamic> get routingData => {...data, 'type': type};

  factory AppNotification.fromJson(Map<String, dynamic> json) => AppNotification(
        id: asInt(json['id']),
        type: asString(json['type']),
        title: asString(json['title']),
        body: asString(json['body']),
        data: asMapOrNull(json['data']) ?? const <String, dynamic>{},
        readAt: asDateTimeOrNull(json['readAt']),
        createdAt: asDateTimeOrNull(json['createdAt']),
      );

  AppNotification markedRead(DateTime at) => AppNotification(
        id: id,
        type: type,
        title: title,
        body: body,
        data: data,
        readAt: readAt ?? at,
        createdAt: createdAt,
      );
}

/// `platform` values for `POST /notifications/devices`.
abstract final class DevicePlatform {
  static const android = 'android';
  static const ios = 'ios';
  static const web = 'web';
}

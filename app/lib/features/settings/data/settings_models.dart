import '../../../core/json/json_utils.dart';

/// `Settings.notificationPrefs`: which push categories are on
/// (docs/architecture.md "Notifications"). In-app notifications are always kept.
class NotificationPrefs {
  const NotificationPrefs({
    required this.messages,
    required this.events,
    required this.social,
    required this.marketing,
  });

  final bool messages;
  final bool events;
  final bool social;
  final bool marketing;

  factory NotificationPrefs.fromJson(Map<String, dynamic> json) => NotificationPrefs(
        messages: asBool(json['messages'], true),
        events: asBool(json['events'], true),
        social: asBool(json['social'], true),
        marketing: asBool(json['marketing']),
      );

  Map<String, dynamic> toJson() =>
      {'messages': messages, 'events': events, 'social': social, 'marketing': marketing};

  NotificationPrefs copyWith({bool? messages, bool? events, bool? social, bool? marketing}) =>
      NotificationPrefs(
        messages: messages ?? this.messages,
        events: events ?? this.events,
        social: social ?? this.social,
        marketing: marketing ?? this.marketing,
      );
}

/// `allowDmsFrom` values.
abstract final class AllowDmsFrom {
  static const everyone = 'everyone';
  static const followers = 'followers';
  static const mutual = 'mutual';
  static const nobody = 'nobody';

  static const values = [everyone, followers, mutual, nobody];

  static const labels = {
    everyone: 'Everyone',
    followers: 'People I follow',
    mutual: 'Mutual follows',
    nobody: 'Nobody',
  };
}

/// `Settings.privacy`.
class PrivacySettings {
  const PrivacySettings({required this.allowDmsFrom, required this.showInSearch});

  final String allowDmsFrom;
  final bool showInSearch;

  factory PrivacySettings.fromJson(Map<String, dynamic> json) => PrivacySettings(
        allowDmsFrom: asString(json['allowDmsFrom'], AllowDmsFrom.everyone),
        showInSearch: asBool(json['showInSearch'], true),
      );

  Map<String, dynamic> toJson() => {'allowDmsFrom': allowDmsFrom, 'showInSearch': showInSearch};

  PrivacySettings copyWith({String? allowDmsFrom, bool? showInSearch}) => PrivacySettings(
        allowDmsFrom: allowDmsFrom ?? this.allowDmsFrom,
        showInSearch: showInSearch ?? this.showInSearch,
      );
}

/// `Settings` (`GET/PUT /me/settings`).
class Settings {
  const Settings({required this.notificationPrefs, required this.privacy});

  final NotificationPrefs notificationPrefs;
  final PrivacySettings privacy;

  factory Settings.fromJson(Map<String, dynamic> json) => Settings(
        notificationPrefs: NotificationPrefs.fromJson(asMap(json['notificationPrefs'])),
        privacy: PrivacySettings.fromJson(asMap(json['privacy'])),
      );

  Map<String, dynamic> toJson() =>
      {'notificationPrefs': notificationPrefs.toJson(), 'privacy': privacy.toJson()};

  Settings copyWith({NotificationPrefs? notificationPrefs, PrivacySettings? privacy}) => Settings(
        notificationPrefs: notificationPrefs ?? this.notificationPrefs,
        privacy: privacy ?? this.privacy,
      );
}

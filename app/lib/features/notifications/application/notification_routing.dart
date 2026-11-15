import '../../../core/json/json_utils.dart';
import '../../../core/router/routes.dart';

/// Maps a notification's `data` (in-app `data` + `type`, or an FCM data
/// payload, where every value is a string) to an app location. Returns null
/// when there is nowhere better to go than the notifications list.
///
/// Pure function: no Flutter, no Firebase, safe to unit test.
///
/// Order matters: types that are about a screen of their own win first, then
/// the most specific id (conversation, post, consultation, event, community,
/// profile).
String? routeForNotificationData(Map<String, dynamic> data) {
  final type = asString(data['type']);

  // Location identity has no id in `data`: route by type.
  if (type.startsWith('location_identity.')) return Routes.locationIdentity;
  if (type.startsWith('organiser.')) return Routes.organiserVerification;
  if (type.startsWith('subscription.')) return Routes.premium;

  final conversationId = _id(data['conversationId']);
  if (conversationId != null) return Routes.conversation(conversationId);

  final postId = _id(data['postId']);
  if (postId != null) return Routes.post(postId);

  if (_id(data['consultationRequestId']) != null || type.startsWith('consultation.')) {
    return Routes.consultations;
  }

  final eventId = _id(data['eventId']);
  if (eventId != null) return Routes.event(eventId);
  if (type.startsWith('registration.')) return Routes.myRegistrations;

  final communityId = _id(data['communityId']);
  if (communityId != null) return Routes.community(communityId);

  final userId = _id(data['userId']);
  if (userId != null) return Routes.profile(userId);

  return null;
}

int? _id(Object? v) {
  final id = asIntOrNull(v);
  return id == null || id <= 0 ? null : id;
}

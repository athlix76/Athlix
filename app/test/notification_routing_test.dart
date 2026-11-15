import 'package:athlix/core/router/routes.dart';
import 'package:athlix/features/notifications/application/notification_routing.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('location identity notifications open the identity screen', () {
    expect(routeForNotificationData({'type': 'location_identity.verified'}), Routes.locationIdentity);
  });

  test('FCM string ids are parsed', () {
    expect(routeForNotificationData({'type': 'chat.message', 'conversationId': '12'}), Routes.conversation(12));
    expect(routeForNotificationData({'type': 'event.updated', 'eventId': '9'}), Routes.event(9));
  });

  test('registration without an event id goes to my registrations', () {
    expect(routeForNotificationData({'type': 'registration.flagged'}), Routes.myRegistrations);
  });

  test('unknown or empty data stays on the list', () {
    expect(routeForNotificationData({}), isNull);
    expect(routeForNotificationData({'type': 'x', 'eventId': '0'}), isNull);
  });
}

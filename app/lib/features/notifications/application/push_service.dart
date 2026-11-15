import 'dart:async';

import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/router/app_router.dart';
import '../../../core/router/routes.dart';
import '../data/notification_models.dart';
import '../data/notifications_api.dart';
import 'notification_routing.dart';

export 'notification_routing.dart' show routeForNotificationData;

/// FCM push for the signed-in user.
///
/// * [start] (after sign-in): asks permission, registers this device's FCM
///   token with `POST /notifications/devices`, follows token refreshes, and
///   opens the right screen when the user taps a push.
/// * [stop] (before logout): `DELETE /notifications/devices` with the token.
///
/// Every Firebase call is wrapped: if Firebase is not configured (no
/// `google-services.json` / `GoogleService-Info.plist`, tests, desktop) the
/// service quietly does nothing. The FCM token is kept in memory only and is
/// **never logged**, printed or stored.
class PushService {
  PushService(this._ref) {
    _ref.onDispose(_cancelSubscriptions);
  }

  final Ref _ref;

  /// Last token registered with the backend; memory only.
  String? _token;
  bool _started = false;
  final List<StreamSubscription<Object?>> _subs = [];

  NotificationsApi get _api => _ref.read(notificationsApiProvider);

  Future<void> start() async {
    if (_started) return;
    _started = true;
    final messaging = await _messaging();
    if (messaging == null) return;

    try {
      await messaging.requestPermission();
    } catch (_) {
      // Permission denied or unsupported: the token may still work (Android < 13).
    }

    try {
      final token = await messaging.getToken();
      if (token != null && token.isNotEmpty) await _register(token);
    } catch (_) {
      // No token (simulator, no Play services): push stays off.
    }

    try {
      _subs.add(messaging.onTokenRefresh.listen((t) => unawaited(_register(t))));
    } catch (_) {}

    try {
      _subs.add(FirebaseMessaging.onMessageOpenedApp.listen(_open));
    } catch (_) {}

    try {
      final initial = await messaging.getInitialMessage();
      if (initial != null) _open(initial);
    } catch (_) {}
  }

  Future<void> stop() async {
    _cancelSubscriptions();
    _started = false;
    var token = _token;
    if (token == null) {
      final messaging = await _messaging();
      try {
        token = await messaging?.getToken();
      } catch (_) {
        token = null;
      }
    }
    _token = null;
    if (token == null || token.isEmpty) return;
    try {
      await _api.unregisterDevice(token);
    } catch (_) {
      // Logout must work offline; the server drops stale tokens when FCM rejects them.
    }
  }

  Future<void> _register(String token) async {
    if (token.isEmpty) return;
    String? version;
    try {
      version = await _ref.read(appVersionProvider.future);
    } catch (_) {
      version = null;
    }
    try {
      await _api.registerDevice(platform: _platform(), token: token, appVersion: version);
      _token = token;
    } catch (_) {
      // Retried on the next start or token refresh. Never log the token.
    }
  }

  void _open(RemoteMessage message) {
    final route = routeForNotificationData(Map<String, dynamic>.from(message.data));
    // After the first frame, so the router has finished its start-up redirect.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      try {
        if (rootNavigatorKey.currentContext == null) return;
        final router = _ref.read(routerProvider);
        // Notifications is a bottom tab: switch to it rather than pushing it.
        route == null ? router.go(Routes.notifications) : router.push(route);
      } catch (_) {}
    });
  }

  void _cancelSubscriptions() {
    for (final s in _subs) {
      unawaited(s.cancel());
    }
    _subs.clear();
  }

  /// FirebaseMessaging if Firebase can be initialised, else null.
  static Future<FirebaseMessaging?> _messaging() async {
    try {
      if (Firebase.apps.isEmpty) await Firebase.initializeApp();
      return FirebaseMessaging.instance;
    } catch (_) {
      return null;
    }
  }

  static String _platform() {
    if (kIsWeb) return DevicePlatform.web;
    return defaultTargetPlatform == TargetPlatform.iOS ? DevicePlatform.ios : DevicePlatform.android;
  }
}

final pushServiceProvider = Provider<PushService>(PushService.new);

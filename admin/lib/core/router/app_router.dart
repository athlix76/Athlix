import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/admins/presentation/admins_screen.dart';
import '../../features/ads/presentation/ads_screen.dart';
import '../../features/audit/presentation/audit_log_screen.dart';
import '../../features/audit/presentation/login_events_screen.dart';
import '../../features/auth/application/session_controller.dart';
import '../../features/auth/presentation/login_screen.dart';
import '../../features/broadcast/presentation/broadcast_screen.dart';
import '../../features/communities/presentation/communities_screen.dart';
import '../../features/config/presentation/config_screen.dart';
import '../../features/dashboard/presentation/dashboard_screen.dart';
import '../../features/districts/presentation/districts_screen.dart';
import '../../features/events/presentation/event_detail_screen.dart';
import '../../features/events/presentation/events_screen.dart';
import '../../features/fee_rules/presentation/fee_rules_screen.dart';
import '../../features/location_review/presentation/location_detail_screen.dart';
import '../../features/location_review/presentation/location_review_screen.dart';
import '../../features/moderation/presentation/conversation_messages_screen.dart';
import '../../features/moderation/presentation/moderation_screen.dart';
import '../../features/organiser_steps/presentation/organiser_steps_screen.dart';
import '../../features/organisers/presentation/organiser_detail_screen.dart';
import '../../features/organisers/presentation/organisers_screen.dart';
import '../../features/payments/presentation/listing_orders_screen.dart';
import '../../features/plans/presentation/plans_screen.dart';
import '../../features/policies/presentation/policies_screen.dart';
import '../../features/reports/presentation/reports_screen.dart';
import '../../features/sports/presentation/sports_screen.dart';
import '../../features/subscriptions/presentation/subscriptions_screen.dart';
import '../../features/users/presentation/user_detail_screen.dart';
import '../../features/users/presentation/users_screen.dart';
import '../../features/verification_requirements/presentation/verification_requirements_screen.dart';
import '../auth/permissions.dart';
import '../widgets/admin_shell.dart';
import '../widgets/permission_gate.dart';
import 'nav.dart';

int _id(GoRouterState s, String name) => int.tryParse(s.pathParameters[name] ?? '') ?? 0;

/// Wraps a screen in the permission gate of its nav item.
Page<void> _gated(GoRouterState s, String navPath, Widget child) {
  final item = navItems.firstWhere((i) => i.path == navPath);
  return NoTransitionPage<void>(key: s.pageKey, child: PermissionGate(anyOf: item.anyOf, child: child));
}

GoRoute _screen(String path, Widget Function(GoRouterState s) build, {List<RouteBase> routes = const []}) {
  return GoRoute(path: path, pageBuilder: (context, s) => _gated(s, path, build(s)), routes: routes);
}

final routerProvider = Provider<GoRouter>((ref) {
  final session = ref.watch(sessionProvider.notifier);
  return GoRouter(
    initialLocation: '/',
    refreshListenable: session,
    redirect: (context, state) {
      final loc = state.uri.path;
      final atLogin = loc == '/login';
      if (!session.isSignedIn) return atLogin ? null : '/login';
      if (atLogin || loc == '/') return homePathFor(session.permissions);
      return null;
    },
    routes: [
      GoRoute(path: '/', builder: (_, __) => const SizedBox.shrink()),
      GoRoute(path: '/login', builder: (_, __) => const LoginScreen()),
      ShellRoute(
        builder: (context, state, child) => AdminShell(location: state.uri.path, child: child),
        routes: [
          GoRoute(path: '/no-access', builder: (_, __) => const ForbiddenScreenBody()),
          _screen('/dashboard', (_) => const DashboardScreen()),
          _screen('/users', (_) => const UsersScreen(), routes: [
            GoRoute(path: ':id', pageBuilder: (c, s) => _gated(s, '/users', UserDetailScreen(userId: _id(s, 'id')))),
          ]),
          _screen('/organisers', (_) => const OrganisersScreen(), routes: [
            GoRoute(path: ':userId', pageBuilder: (c, s) => _gated(s, '/organisers', OrganiserDetailScreen(userId: _id(s, 'userId')))),
          ]),
          _screen('/location-review', (_) => const LocationReviewScreen(), routes: [
            GoRoute(path: ':id', pageBuilder: (c, s) => _gated(s, '/location-review', LocationDetailScreen(identityId: _id(s, 'id')))),
          ]),
          _screen('/admins', (_) => const AdminsScreen()),
          _screen('/events', (_) => const EventsScreen(), routes: [
            GoRoute(path: ':id', pageBuilder: (c, s) => _gated(s, '/events', EventDetailScreen(eventId: _id(s, 'id')))),
          ]),
          _screen('/reports', (_) => const ReportsScreen()),
          _screen('/moderation', (_) => const ModerationScreen(), routes: [
            GoRoute(
              path: 'conversations/:id',
              pageBuilder: (c, s) => NoTransitionPage<void>(
                key: s.pageKey,
                child: PermissionGate(
                  anyOf: const [Perm.moderationReadMessages],
                  // The audit reason is passed in memory (route extra), never in the URL.
                  child: ConversationMessagesScreen(conversationId: _id(s, 'id'), reason: s.extra is String ? s.extra as String : null),
                ),
              ),
            ),
          ]),
          _screen('/communities', (_) => const CommunitiesScreen()),
          _screen('/ads', (_) => const AdsScreen()),
          _screen('/broadcast', (_) => const BroadcastScreen()),
          _screen('/listing-orders', (_) => const ListingOrdersScreen()),
          _screen('/fee-rules', (_) => const FeeRulesScreen()),
          _screen('/plans', (_) => const PlansScreen()),
          _screen('/subscriptions', (_) => const SubscriptionsScreen()),
          _screen('/config', (_) => const ConfigScreen()),
          _screen('/policies', (_) => const PoliciesScreen()),
          _screen('/verification-requirements', (_) => const VerificationRequirementsScreen()),
          _screen('/organiser-steps', (_) => const OrganiserStepsScreen()),
          _screen('/sports', (_) => const SportsScreen()),
          _screen('/districts', (_) => const DistrictsScreen()),
          _screen('/audit-log', (_) => const AuditLogScreen()),
          _screen('/login-events', (_) => const LoginEventsScreen()),
        ],
      ),
    ],
  );
});

/// Body of `/no-access`: signed in, but the role grants no screen.
class ForbiddenScreenBody extends StatelessWidget {
  const ForbiddenScreenBody({super.key});

  @override
  Widget build(BuildContext context) => const Center(
        child: Padding(
          padding: EdgeInsets.all(24),
          child: Text('Your role has no permissions yet. Ask a super admin to update it.'),
        ),
      );
}

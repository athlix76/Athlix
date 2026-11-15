import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/auth/presentation/otp_screen.dart';
import '../../features/auth/presentation/phone_screen.dart';
import '../../features/chat/presentation/chat_members_screen.dart';
import '../../features/chat/presentation/chat_thread_screen.dart';
import '../../features/chat/presentation/conversations_screen.dart';
import '../../features/communities/presentation/communities_screen.dart';
import '../../features/communities/presentation/community_detail_screen.dart';
import '../../features/communities/presentation/community_form_screen.dart';
import '../../features/communities/presentation/community_manage_screen.dart';
import '../../features/consultations/presentation/coach_consultation_screen.dart';
import '../../features/consultations/presentation/consultation_offer_screen.dart';
import '../../features/consultations/presentation/consultations_screen.dart';
import '../../features/events/presentation/event_detail_screen.dart';
import '../../features/events/presentation/event_form_screen.dart';
import '../../features/events/presentation/events_screen.dart';
import '../../features/events/presentation/my_events_screen.dart';
import '../../features/feed/presentation/feed_screen.dart';
import '../../features/feed/presentation/post_composer_screen.dart';
import '../../features/feed/presentation/post_detail_screen.dart';
import '../../features/feed/presentation/user_posts_screen.dart';
import '../../features/follow/presentation/blocked_users_screen.dart';
import '../../features/follow/presentation/follow_list_screen.dart';
import '../../features/guardian/presentation/guardian_otp_screen.dart';
import '../../features/guardian/presentation/guardian_screen.dart';
import '../../features/home/presentation/home_screen.dart';
import '../../features/home/presentation/home_shell.dart';
import '../../features/listing_payment/presentation/listing_checkout_screen.dart';
import '../../features/location_identity/presentation/location_identity_screen.dart';
import '../../features/location_identity/presentation/location_identity_submit_screen.dart';
import '../../features/notifications/presentation/notifications_screen.dart';
import '../../features/onboarding/presentation/profile_setup_screen.dart';
import '../../features/onboarding/presentation/role_screen.dart';
import '../../features/organiser/presentation/organiser_verification_screen.dart';
import '../../features/policies/presentation/policy_accept_screen.dart';
import '../../features/policies/presentation/policy_screen.dart';
import '../../features/premium/presentation/premium_screen.dart';
import '../../features/profile/presentation/achievements_screen.dart';
import '../../features/profile/presentation/edit_profile_screen.dart';
import '../../features/profile/presentation/my_profile_screen.dart';
import '../../features/profile/presentation/profile_screen.dart';
import '../../features/profile/presentation/profile_viewers_screen.dart';
import '../../features/profile/presentation/stats_screen.dart';
import '../../features/registrations/presentation/my_registrations_screen.dart';
import '../../features/registrations/presentation/organiser_registrations_screen.dart';
import '../../features/registrations/presentation/register_screen.dart';
import '../../features/search/presentation/people_search_screen.dart';
import '../../features/search/presentation/search_screen.dart';
import '../../features/settings/presentation/settings_screen.dart';
import '../../features/startup/presentation/force_update_screen.dart';
import '../../features/startup/presentation/maintenance_screen.dart';
import '../../features/startup/presentation/splash_screen.dart';
import '../../features/startup/presentation/suspended_screen.dart';
import '../../features/verification/presentation/verification_result_screen.dart';
import '../../features/verification/presentation/verification_screen.dart';
import '../config/config_providers.dart';
import '../session/session_controller.dart';
import 'redirect.dart';
import 'routes.dart';

final rootNavigatorKey = GlobalKey<NavigatorState>(debugLabel: 'root');

int _id(GoRouterState s, [String name = 'id']) => int.tryParse(s.pathParameters[name] ?? '') ?? 0;

GoRoute _route(String path, Widget Function(GoRouterState s) build) =>
    GoRoute(path: path, builder: (context, state) => build(state));

/// The app router. Re-evaluates [computeRedirect] whenever the session or
/// the start-up gate changes.
final routerProvider = Provider<GoRouter>((ref) {
  final refresh = ValueNotifier<int>(0);
  ref.listen(sessionProvider, (_, __) => refresh.value++);
  ref.listen(appGateProvider, (_, __) => refresh.value++);

  final router = GoRouter(
    navigatorKey: rootNavigatorKey,
    initialLocation: Routes.splash,
    refreshListenable: refresh,
    debugLogDiagnostics: false,
    redirect: (context, state) {
      final gate = ref.read(appGateProvider);
      return computeRedirect(
        state.uri,
        RedirectState(
          gateReady: gate.ready,
          forceUpdate: gate.forceUpdate,
          maintenance: gate.maintenance,
          session: ref.read(sessionProvider),
        ),
      );
    },
    errorBuilder: (context, state) => Scaffold(
      appBar: AppBar(),
      body: const Center(child: Text('Page not found.')),
    ),
    routes: [
      // Start-up gates.
      _route(Routes.splash, (_) => const SplashScreen()),
      _route(Routes.forceUpdate, (_) => const ForceUpdateScreen()),
      _route(Routes.maintenance, (_) => const MaintenanceScreen()),
      _route(Routes.suspended, (_) => const SuspendedScreen()),
      GoRoute(
        path: Routes.deepLinkResult,
        redirect: (context, state) => state.uri.hasQuery
            ? '${Routes.verificationResult}?${state.uri.query}'
            : Routes.verificationResult,
      ),

      // Sign-in and onboarding.
      _route(Routes.authPhone, (_) => const PhoneScreen()),
      _route(Routes.authOtp, (_) => const OtpScreen()),
      _route(Routes.onboardingPolicies, (_) => const PolicyAcceptScreen()),
      _route(Routes.onboardingRole, (_) => const RoleScreen()),
      _route(Routes.onboardingProfile, (_) => const ProfileSetupScreen()),
      _route(Routes.verification, (_) => const VerificationScreen()),
      _route(
        Routes.verificationResult,
        (s) => VerificationResultScreen(
          status: s.uri.queryParameters['status'],
          reason: s.uri.queryParameters['reason'],
        ),
      ),
      _route(Routes.guardian, (_) => const GuardianScreen()),
      _route(Routes.guardianOtp, (_) => const GuardianOtpScreen()),
      _route('/policy/:docType', (s) => PolicyScreen(docType: s.pathParameters['docType'] ?? '')),

      // Bottom tabs.
      StatefulShellRoute.indexedStack(
        builder: (context, state, shell) => HomeShell(navigationShell: shell),
        branches: [
          StatefulShellBranch(routes: [_route(Routes.home, (_) => const HomeScreen())]),
          StatefulShellBranch(routes: [_route(Routes.events, (_) => const EventsScreen())]),
          StatefulShellBranch(routes: [_route(Routes.search, (_) => const SearchScreen())]),
          StatefulShellBranch(routes: [_route(Routes.notifications, (_) => const NotificationsScreen())]),
          StatefulShellBranch(routes: [_route(Routes.me, (_) => const MyProfileScreen())]),
        ],
      ),

      // Events, listing and registrations (`/events/new` before `/events/:id`).
      _route(Routes.eventNew, (_) => const EventFormScreen()),
      _route('/events/:id', (s) => EventDetailScreen(eventId: _id(s))),
      _route('/events/:id/edit', (s) => EventFormScreen(eventId: _id(s))),
      _route('/events/:id/checkout', (s) => ListingCheckoutScreen(eventId: _id(s))),
      _route('/events/:id/registrations', (s) => OrganiserRegistrationsScreen(eventId: _id(s))),
      _route('/events/:id/register', (s) => RegisterScreen(eventId: _id(s))),
      _route(Routes.myEvents, (_) => const MyEventsScreen()),
      _route(Routes.myRegistrations, (_) => const MyRegistrationsScreen()),

      // Search and people (`/search` itself is a bottom tab).
      _route(Routes.searchPeople, (_) => const PeopleSearchScreen()),
      _route('/profile/:id', (s) => ProfileScreen(userId: _id(s))),
      _route('/profile/:id/followers', (s) => FollowListScreen(userId: _id(s), following: false)),
      _route('/profile/:id/following', (s) => FollowListScreen(userId: _id(s), following: true)),
      _route('/profile/:id/stats', (s) => StatsScreen(userId: _id(s))),
      _route('/profile/:id/achievements', (s) => AchievementsScreen(userId: _id(s))),
      _route('/profile/:id/posts', (s) => UserPostsScreen(userId: _id(s))),
      _route(Routes.meEdit, (_) => const EditProfileScreen()),
      _route(Routes.meViewers, (_) => const ProfileViewersScreen()),

      // Chat (opened from the icon in the Home app bar) and communities.
      _route(Routes.chat, (_) => const ConversationsScreen()),
      _route('/chat/:id', (s) => ChatThreadScreen(conversationId: _id(s))),
      _route('/chat/:id/members', (s) => ChatMembersScreen(conversationId: _id(s))),
      _route(Routes.communities, (_) => const CommunitiesScreen()),
      _route(Routes.communityNew, (_) => const CommunityFormScreen()),
      _route('/communities/:id', (s) => CommunityDetailScreen(communityId: _id(s))),
      _route('/communities/:id/edit', (s) => CommunityFormScreen(communityId: _id(s))),
      _route('/communities/:id/manage', (s) => CommunityManageScreen(communityId: _id(s))),

      // Feed. Not linked from the navigation any more (removed from the bottom tabs);
      // the route stays registered so the screen and post links keep working.
      _route(Routes.feed, (_) => const FeedScreen()),
      _route(
        Routes.postNew,
        (s) => PostComposerScreen(eventId: int.tryParse(s.uri.queryParameters['eventId'] ?? '')),
      ),
      _route('/posts/:id', (s) => PostDetailScreen(postId: _id(s))),

      // Everything else (`/notifications` is a bottom tab).
      _route(Routes.premium, (_) => const PremiumScreen()),
      _route(Routes.consultations, (_) => const ConsultationsScreen()),
      _route(Routes.consultationOffer, (_) => const ConsultationOfferScreen()),
      _route('/coaches/:id/consultation', (s) => CoachConsultationScreen(coachId: _id(s))),
      _route(Routes.organiserVerification, (_) => const OrganiserVerificationScreen()),
      _route(Routes.settings, (_) => const SettingsScreen()),
      _route(Routes.blocked, (_) => const BlockedUsersScreen()),
      _route(Routes.locationIdentity, (_) => const LocationIdentityScreen()),
      _route(
        Routes.locationIdentitySubmitPath,
        (s) => LocationIdentitySubmitScreen(
          identityType: s.uri.queryParameters['type'] == 'native' ? 'native' : 'residence',
        ),
      ),
    ],
  );

  ref.onDispose(() {
    router.dispose();
    refresh.dispose();
  });
  return router;
});

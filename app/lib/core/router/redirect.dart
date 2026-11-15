import '../models/me.dart';
import '../session/session_controller.dart';
import 'routes.dart';

/// Everything the redirect needs, as plain values (easy to unit test).
class RedirectState {
  const RedirectState({
    required this.gateReady,
    required this.forceUpdate,
    required this.maintenance,
    required this.session,
  });

  final bool gateReady;
  final bool forceUpdate;
  final bool maintenance;
  final SessionState session;
}

/// Screen for each unfinished onboarding step (`Me.onboardingStep`, or
/// `fields.nextStep` of a 403 `ONBOARDING_REQUIRED`).
String routeForStep(String step) {
  switch (step) {
    case OnboardingStep.role:
      return Routes.onboardingRole;
    case OnboardingStep.verification:
      return Routes.verification;
    case OnboardingStep.guardian:
      return Routes.guardian;
    case OnboardingStep.profile:
      return Routes.onboardingProfile;
    case OnboardingStep.done:
      return Routes.home;
    default:
      return Routes.onboardingRole;
  }
}

bool _under(String path, String base) => path == base || path.startsWith('$base/');

const _gateScreens = {Routes.splash, Routes.forceUpdate, Routes.maintenance, Routes.suspended};

/// The single routing rule of the app, in priority order:
///
/// 1. DigiLocker deep link `/result` → `/verification/result` (query kept).
/// 2. Bootstrap not loaded → splash. Force update / maintenance screens win.
/// 3. Session unknown → splash. Signed out → only `/auth/*` and `/policy/*`.
/// 4. Suspended → suspended screen.
/// 5. Pending policies → accept screen (policy documents stay readable).
/// 6. Onboarding not `done` → the screen for `onboardingStep` (and its sub-pages,
///    the verification result, policies and settings).
/// 7. Onboarded users never see gate, auth or onboarding screens again
///    (except `/verification/result`, which shows the outcome).
///
/// Returns the new location, or null to stay.
String? computeRedirect(Uri uri, RedirectState s) {
  final path = uri.path.isEmpty ? '/' : uri.path;

  if (path == Routes.deepLinkResult) {
    return uri.hasQuery ? '${Routes.verificationResult}?${uri.query}' : Routes.verificationResult;
  }

  String? to(String target) => path == target ? null : target;

  if (!s.gateReady) return to(Routes.splash);
  if (s.forceUpdate) return to(Routes.forceUpdate);
  if (s.maintenance) return to(Routes.maintenance);

  final session = s.session;
  switch (session.status) {
    case SessionStatus.unknown:
      return to(Routes.splash);
    case SessionStatus.signedOut:
      if (_under(path, '/auth') || _under(path, '/policy')) return null;
      return Routes.authPhone;
    case SessionStatus.signedIn:
      break;
  }

  final me = session.me;
  if (me == null) return to(Routes.splash);

  if (me.isSuspended) return to(Routes.suspended);

  if (me.pendingPolicies.isNotEmpty) {
    if (path == Routes.onboardingPolicies || _under(path, '/policy')) return null;
    return Routes.onboardingPolicies;
  }

  if (!me.isOnboarded) {
    final target = routeForStep(me.onboardingStep);
    if (_under(path, target) ||
        path == Routes.verificationResult ||
        _under(path, '/policy') ||
        _under(path, Routes.settings)) {
      return null;
    }
    return target;
  }

  final onboardingOnly = _gateScreens.contains(path) ||
      path == '/' ||
      _under(path, '/auth') ||
      _under(path, '/onboarding') ||
      _under(path, Routes.guardian) ||
      path == Routes.verification;
  return onboardingOnly ? Routes.home : null;
}

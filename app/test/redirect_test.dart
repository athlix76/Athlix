import 'package:athlix/core/models/me.dart';
import 'package:athlix/core/router/redirect.dart';
import 'package:athlix/core/router/routes.dart';
import 'package:athlix/core/session/session_controller.dart';
import 'package:flutter_test/flutter_test.dart';

Me me({
  String step = OnboardingStep.done,
  String status = 'active',
  List<Map<String, dynamic>> pending = const [],
}) =>
    Me.fromJson({
      'id': 7,
      'displayName': 'Asha',
      'mobileMasked': '+91 98XXXXXX10',
      'primaryRole': 'player',
      'roles': ['player'],
      'status': status,
      'onboardingStep': step,
      'verificationStatus': 'verified',
      'isMinor': false,
      'guardianConsentStatus': 'not_required',
      'pendingPolicies': pending,
    });

RedirectState signedIn(Me m) => RedirectState(
      gateReady: true,
      forceUpdate: false,
      maintenance: false,
      session: SessionState(status: SessionStatus.signedIn, me: m),
    );

const signedOut = RedirectState(
  gateReady: true,
  forceUpdate: false,
  maintenance: false,
  session: SessionState.signedOut,
);

String? go(String location, RedirectState s) => computeRedirect(Uri.parse(location), s);

void main() {
  group('gates', () {
    test('bootstrap not loaded goes to splash (and stays)', () {
      const s = RedirectState(
          gateReady: false, forceUpdate: false, maintenance: false, session: SessionState.unknown);
      expect(go('/home', s), Routes.splash);
      expect(go(Routes.splash, s), isNull);
    });

    test('force update beats maintenance', () {
      final s = RedirectState(
          gateReady: true, forceUpdate: true, maintenance: true, session: signedIn(me()).session);
      expect(go('/home', s), Routes.forceUpdate);
    });

    test('maintenance', () {
      final s = RedirectState(
          gateReady: true, forceUpdate: false, maintenance: true, session: signedIn(me()).session);
      expect(go('/events', s), Routes.maintenance);
    });

    test('unknown session goes to splash', () {
      const s = RedirectState(
          gateReady: true, forceUpdate: false, maintenance: false, session: SessionState.unknown);
      expect(go('/home', s), Routes.splash);
    });
  });

  group('signed out', () {
    test('only auth and policy pages are reachable', () {
      expect(go('/home', signedOut), Routes.authPhone);
      expect(go(Routes.authPhone, signedOut), isNull);
      expect(go(Routes.authOtp, signedOut), isNull);
      expect(go('/policy/terms', signedOut), isNull);
    });
  });

  group('signed in', () {
    test('suspended account', () {
      expect(go('/home', signedIn(me(status: 'suspended'))), Routes.suspended);
    });

    test('pending policies first', () {
      final s = signedIn(me(pending: [
        {'id': 1, 'docType': 'terms', 'version': 2, 'title': 'Terms'}
      ]));
      expect(go('/home', s), Routes.onboardingPolicies);
      expect(go(Routes.onboardingPolicies, s), isNull);
      expect(go('/policy/terms', s), isNull);
    });

    test('each onboarding step has its screen', () {
      expect(go('/home', signedIn(me(step: OnboardingStep.role))), Routes.onboardingRole);
      expect(go('/home', signedIn(me(step: OnboardingStep.verification))), Routes.verification);
      expect(go('/home', signedIn(me(step: OnboardingStep.guardian))), Routes.guardian);
      expect(go('/home', signedIn(me(step: OnboardingStep.profile))), Routes.onboardingProfile);
    });

    test('onboarding allows its sub pages, result, policies and settings', () {
      final s = signedIn(me(step: OnboardingStep.guardian));
      expect(go(Routes.guardianOtp, s), isNull);
      expect(go(Routes.verificationResult, s), isNull);
      expect(go('/policy/privacy', s), isNull);
      expect(go(Routes.settings, s), isNull);
    });

    test('onboarded users skip gate, auth and onboarding screens', () {
      final s = signedIn(me());
      expect(go(Routes.splash, s), Routes.home);
      expect(go('/', s), Routes.home);
      expect(go(Routes.authPhone, s), Routes.home);
      expect(go(Routes.onboardingRole, s), Routes.home);
      expect(go(Routes.verification, s), Routes.home);
      expect(go(Routes.guardian, s), Routes.home);
      expect(go('/events', s), isNull);
      expect(go(Routes.verificationResult, s), isNull);
      expect(go('/settings/location', s), isNull);
    });
  });

  test('DigiLocker deep link keeps its query', () {
    expect(go('/result?status=success', signedOut), '${Routes.verificationResult}?status=success');
    expect(go('/result', signedOut), Routes.verificationResult);
  });

  test('routeForStep falls back to role', () {
    expect(routeForStep('something_new'), Routes.onboardingRole);
    expect(routeForStep(OnboardingStep.done), Routes.home);
  });
}

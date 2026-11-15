import '../../../core/api/api_exception.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/router/routes.dart';
import '../data/eligibility_models.dart';
import '../data/location_identity_models.dart';

/// Pure mapping from the server's eligibility answer to what the UI shows.
/// The server decides eligibility (`platform/eligibility`); the app only
/// renders `myEligibility` / `NOT_ELIGIBLE` and never re-evaluates rules.

/// User-facing sentence for a `NOT_ELIGIBLE` reason.
String eligibilityReasonText(String? reason) => switch (reason) {
      EligibilityReason.identityMissing =>
        'This event is limited to players from a specific place. Add your location identity to take part.',
      EligibilityReason.identityPending =>
        'Your location identity is being reviewed. You can take part once it is verified.',
      EligibilityReason.identityExpired =>
        'Your residence proof has expired. Renew it to take part.',
      EligibilityReason.wrongLocation =>
        'This event is open only to players from another district or state.',
      _ => 'You cannot take part in this event.',
    };

/// Button label for an action, or null when there is nothing to do.
String? eligibilityActionLabel(String? action) => switch (action) {
      EligibilityAction.verifyNative => 'Verify native place',
      EligibilityAction.verifyResidence => 'Verify residence',
      EligibilityAction.renewResidence => 'Renew residence',
      EligibilityAction.waitForReview => 'View review status',
      _ => null,
    };

/// Route the action button opens, or null for `none`.
String? eligibilityActionRoute(String? action) => switch (action) {
      EligibilityAction.verifyNative => Routes.locationIdentitySubmit(IdentityType.native),
      EligibilityAction.verifyResidence ||
      EligibilityAction.renewResidence =>
        Routes.locationIdentitySubmit(IdentityType.residence),
      EligibilityAction.waitForReview => Routes.locationIdentity,
      _ => null,
    };

/// What an event card / detail screen shows for the caller.
class EligibilityView {
  const EligibilityView({
    required this.eligible,
    required this.greyed,
    this.tag,
    this.pending = false,
    this.reasonText,
    this.ctaLabel,
    this.ctaRoute,
  });

  final bool eligible;

  /// Card is dimmed (only shown at all with `showAll=true`).
  final bool greyed;

  /// Chip text from the server (`myEligibility.tag`), e.g. "Native: Thanjavur".
  final String? tag;

  /// Eligible on a pending identity (`requireVerified: false`).
  final bool pending;
  final String? reasonText;

  /// Short card button ("Verify now"); null when the user cannot fix it.
  final String? ctaLabel;
  final String? ctaRoute;

  /// Null [me] means the feature is off or the event is open: always eligible.
  factory EligibilityView.from(MyEligibility? me) {
    if (me == null) return const EligibilityView(eligible: true, greyed: false);
    final route = me.eligible ? null : eligibilityActionRoute(me.action);
    return EligibilityView(
      eligible: me.eligible,
      greyed: !me.eligible,
      tag: me.tag,
      pending: me.pending,
      reasonText: me.eligible ? null : eligibilityReasonText(me.reason),
      ctaLabel: route == null
          ? null
          : me.action == EligibilityAction.waitForReview
              ? 'In review'
              : me.action == EligibilityAction.renewResidence
                  ? 'Renew now'
                  : 'Verify now',
      ctaRoute: route,
    );
  }
}

/// Reason / action carried by a 403 `NOT_ELIGIBLE`.
class NotEligibleInfo {
  const NotEligibleInfo({this.reason, this.action = EligibilityAction.none});

  final String? reason;
  final String action;

  String get text => eligibilityReasonText(reason);
  String? get actionLabel => eligibilityActionLabel(action);
  String? get actionRoute => eligibilityActionRoute(action);

  /// Null when [error] is not a `NOT_ELIGIBLE` error.
  static NotEligibleInfo? fromError(Object? error) {
    if (error is! ApiException || error.code != ApiErrorCode.notEligible) return null;
    return NotEligibleInfo(
      reason: asStringOrNull(error.fields['reason']),
      action: asString(error.fields['action'], EligibilityAction.none),
    );
  }
}

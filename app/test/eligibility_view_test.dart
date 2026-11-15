import 'package:athlix/core/api/api_exception.dart';
import 'package:athlix/core/location/device_location.dart';
import 'package:athlix/core/router/routes.dart';
import 'package:athlix/features/events/data/event_models.dart';
import 'package:athlix/features/events/presentation/event_discovery_bar.dart';
import 'package:athlix/features/location_identity/application/eligibility_view.dart';
import 'package:athlix/features/location_identity/application/location_identity_controller.dart';
import 'package:athlix/features/location_identity/data/eligibility_models.dart';
import 'package:flutter_test/flutter_test.dart';

MyEligibility my(Map<String, dynamic> json) => MyEligibility.fromJson(json);

void main() {
  group('EligibilityView.from', () {
    test('feature off / open event (null) is eligible and not greyed', () {
      final v = EligibilityView.from(null);
      expect(v.eligible, isTrue);
      expect(v.greyed, isFalse);
      expect(v.ctaRoute, isNull);
      expect(v.reasonText, isNull);
    });

    test('eligible via native shows the tag only', () {
      final v = EligibilityView.from(my({
        'eligible': true,
        'via': 'native',
        'pending': false,
        'tag': 'Native: Thanjavur',
        'action': 'none',
      }));
      expect(v.eligible, isTrue);
      expect(v.greyed, isFalse);
      expect(v.tag, 'Native: Thanjavur');
      expect(v.ctaLabel, isNull);
      expect(v.reasonText, isNull);
    });

    test('eligible via residence', () {
      final v = EligibilityView.from(my({
        'eligible': true,
        'via': 'residence',
        'tag': 'Residence: Chennai',
        'action': 'none',
      }));
      expect(v.greyed, isFalse);
      expect(v.tag, 'Residence: Chennai');
    });

    test('either rule satisfied by one identity is eligible', () {
      final v = EligibilityView.from(my({'eligible': true, 'via': 'native', 'action': 'none'}));
      expect(v.eligible, isTrue);
    });

    test('state scope eligible', () {
      final v = EligibilityView.from(
          my({'eligible': true, 'via': 'residence', 'tag': 'Residence: Tamil Nadu', 'action': 'none'}));
      expect(v.tag, 'Residence: Tamil Nadu');
      expect(v.greyed, isFalse);
    });

    test('requireVerified=false with a pending identity is eligible but pending', () {
      final v = EligibilityView.from(
          my({'eligible': true, 'via': 'native', 'pending': true, 'tag': 'Native (in review)', 'action': 'none'}));
      expect(v.eligible, isTrue);
      expect(v.pending, isTrue);
      expect(v.greyed, isFalse);
      expect(v.ctaRoute, isNull);
    });

    test('missing identity: greyed with "Verify now" to the native form', () {
      final v = EligibilityView.from(
          my({'eligible': false, 'reason': 'identity_missing', 'action': 'verify_native'}));
      expect(v.greyed, isTrue);
      expect(v.ctaLabel, 'Verify now');
      expect(v.ctaRoute, Routes.locationIdentitySubmit('native'));
      expect(v.reasonText, contains('location identity'));
    });

    test('wrong location: greyed with no action', () {
      final v = EligibilityView.from(my({'eligible': false, 'reason': 'wrong_location', 'action': 'none'}));
      expect(v.greyed, isTrue);
      expect(v.ctaLabel, isNull);
      expect(v.ctaRoute, isNull);
      expect(v.reasonText, contains('another district or state'));
    });

    test('expired residence: "Renew now" to the residence form', () {
      final v = EligibilityView.from(
          my({'eligible': false, 'reason': 'identity_expired', 'action': 'renew_residence'}));
      expect(v.ctaLabel, 'Renew now');
      expect(v.ctaRoute, Routes.locationIdentitySubmit('residence'));
    });

    test('pending with requireVerified=true: "In review" to the status screen', () {
      final v = EligibilityView.from(
          my({'eligible': false, 'pending': true, 'reason': 'identity_pending', 'action': 'wait_for_review'}));
      expect(v.greyed, isTrue);
      expect(v.ctaLabel, 'In review');
      expect(v.ctaRoute, Routes.locationIdentity);
    });

    test('missing fields default to eligible', () {
      final v = EligibilityView.from(my({}));
      expect(v.eligible, isTrue);
      expect(v.greyed, isFalse);
    });
  });

  group('reason and action mapping', () {
    test('every reason has its own sentence; unknown has a fallback', () {
      final texts = {
        for (final r in [
          EligibilityReason.identityMissing,
          EligibilityReason.identityPending,
          EligibilityReason.identityExpired,
          EligibilityReason.wrongLocation,
        ])
          eligibilityReasonText(r)
      };
      expect(texts.length, 4);
      expect(eligibilityReasonText('new_reason'), isNotEmpty);
      expect(eligibilityReasonText(null), isNotEmpty);
    });

    test('action -> label and route', () {
      expect(eligibilityActionLabel('verify_native'), 'Verify native place');
      expect(eligibilityActionLabel('verify_residence'), 'Verify residence');
      expect(eligibilityActionLabel('renew_residence'), 'Renew residence');
      expect(eligibilityActionLabel('wait_for_review'), 'View review status');
      expect(eligibilityActionLabel('none'), isNull);
      expect(eligibilityActionLabel(null), isNull);

      expect(eligibilityActionRoute('verify_native'), '/settings/location/submit?type=native');
      expect(eligibilityActionRoute('verify_residence'), '/settings/location/submit?type=residence');
      expect(eligibilityActionRoute('renew_residence'), '/settings/location/submit?type=residence');
      expect(eligibilityActionRoute('wait_for_review'), '/settings/location');
      expect(eligibilityActionRoute('none'), isNull);
      expect(eligibilityActionRoute('something_new'), isNull);
    });
  });

  group('NotEligibleInfo.fromError', () {
    test('reads reason and action from a 403 NOT_ELIGIBLE', () {
      final info = NotEligibleInfo.fromError(const ApiException(
        code: ApiErrorCode.notEligible,
        message: 'Not eligible',
        fields: {'reason': 'identity_missing', 'action': 'verify_residence'},
        statusCode: 403,
      ));
      expect(info, isNotNull);
      expect(info!.reason, 'identity_missing');
      expect(info.actionLabel, 'Verify residence');
      expect(info.actionRoute, Routes.locationIdentitySubmit('residence'));
    });

    test('missing action defaults to none', () {
      final info = NotEligibleInfo.fromError(
          const ApiException(code: ApiErrorCode.notEligible, message: 'x', fields: {'reason': 'wrong_location'}));
      expect(info!.action, EligibilityAction.none);
      expect(info.actionRoute, isNull);
    });

    test('other errors are not eligibility errors', () {
      expect(NotEligibleInfo.fromError(const ApiException(code: ApiErrorCode.forbidden, message: 'x')), isNull);
      expect(NotEligibleInfo.fromError(StateError('x')), isNull);
      expect(NotEligibleInfo.fromError(null), isNull);
    });
  });

  group('SubmitProblem.from', () {
    test('LIMIT_REACHED is the residence cooldown with its date', () {
      final p = SubmitProblem.from(const ApiException(
        code: ApiErrorCode.limitReached,
        message: 'x',
        fields: {'nextChangeAt': '2027-02-01T00:00:00Z'},
      ));
      expect(p, isA<CooldownProblem>());
      expect((p! as CooldownProblem).nextChangeAt, DateTime.utc(2027, 2, 1));
    });

    test('CONFLICT means one is already pending', () {
      expect(SubmitProblem.from(const ApiException(code: ApiErrorCode.conflict, message: 'x')),
          isA<PendingExistsProblem>());
    });

    test('other errors are not submit problems', () {
      expect(SubmitProblem.from(const ApiException(code: ApiErrorCode.validationFailed, message: 'x')), isNull);
      expect(SubmitProblem.from(StateError('x')), isNull);
    });
  });

  group('EligibilityInput.toJson', () {
    test('open sends no scope or ids', () {
      final j = EligibilityInput(type: 'open', scope: 'district', districtId: 1, stateId: 2).toJson();
      expect(j, {'type': 'open', 'scope': null, 'districtId': null, 'stateId': null, 'requireVerified': true});
    });

    test('district scope sends only the district', () {
      final j = EligibilityInput(type: 'native', scope: 'district', districtId: 603, stateId: 33).toJson();
      expect(j['districtId'], 603);
      expect(j['stateId'], isNull);
    });

    test('state scope sends only the state', () {
      final j = EligibilityInput(type: 'either', scope: 'state', districtId: 603, stateId: 33, requireVerified: false)
          .toJson();
      expect(j['districtId'], isNull);
      expect(j['stateId'], 33);
      expect(j['requireVerified'], isFalse);
    });

    test('fromRule restores ids and derives the state from the district', () {
      final rule = EligibilityRule.fromJson({
        'type': 'residence',
        'scope': 'district',
        'district': {'id': 603, 'stateId': 33, 'name': 'Thanjavur'},
        'requireVerified': true,
      });
      final input = EligibilityInput.fromRule(rule);
      expect(input.districtId, 603);
      expect(input.stateId, 33);
      expect(EligibilityInput.fromRule(null).type, EligibilityType.open);
    });

    test('preview query carries the category', () {
      final q = EligibilityInput(type: 'native', scope: 'state', stateId: 33).toPreviewQuery('government');
      expect(q['category'], 'government');
      expect(q['stateId'], 33);
    });
  });

  group('discovery filters', () {
    test('allowedRadii respects the configured maximum', () {
      expect(allowedRadii(null), radiusChoicesKm);
      expect(allowedRadii(50), [10, 25, 50]);
      expect(allowedRadii(5), isEmpty);
    });

    test('distance params only when both location and radius are set', () {
      const f = EventFilters();
      expect(f.toQuery()['lat'], isNull);
      expect(f.toQuery()['showAll'], isNull);

      final near = f.copyWith(near: () => const LatLng(10.78, 79.13), radiusKm: () => 25, showAll: true);
      final q = near.toQuery();
      expect(q['lat'], 10.78);
      expect(q['lng'], 79.13);
      expect(q['radiusKm'], 25);
      expect(q['sort'], 'distance');
      expect(q['showAll'], isTrue);

      final cleared = near.copyWith(near: () => null);
      expect(cleared.toQuery()['sort'], isNull);
      expect(cleared.toQuery()['radiusKm'], isNull);
    });
  });
}

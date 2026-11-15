import 'package:athlix/core/models/me.dart';
import 'package:athlix/features/chat/data/chat_models.dart';
import 'package:athlix/features/events/data/event_models.dart';
import 'package:athlix/features/listing_payment/data/listing_models.dart';
import 'package:athlix/features/location_identity/data/eligibility_models.dart';
import 'package:athlix/features/registrations/data/registration_models.dart';
import 'package:flutter_test/flutter_test.dart';

/// fromJson(toJson(x)) must give the same JSON again.
void expectStable<T>(Map<String, dynamic> json, T Function(Map<String, dynamic>) fromJson,
    Map<String, dynamic> Function(T) toJson) {
  final first = toJson(fromJson(json));
  final second = toJson(fromJson(first));
  expect(second, first);
}

final meJson = <String, dynamic>{
  'id': 42,
  'displayName': 'Asha',
  'mobileMasked': '+91 98XXXXXX10',
  'primaryRole': 'organiser',
  'roles': ['player', 'organiser'],
  'status': 'active',
  'onboardingStep': 'done',
  'verificationStatus': 'verified',
  'isMinor': false,
  'guardianConsentStatus': 'not_required',
  'photoUrl': null,
  'storeAccountToken': '7f1c0c1e-0000-4000-8000-000000000001',
  'pendingPolicies': [
    {'id': 3, 'docType': 'terms', 'version': 2, 'title': 'Terms of use'}
  ],
  'createdAt': '2026-01-02T03:04:05Z',
};

final quoteJson = <String, dynamic>{
  'feeRuleId': 1,
  'ruleName': 'Default',
  'entryFeePaise': 50000,
  'maxSlots': 64,
  'grossEntryPaise': 3200000,
  'percentBp': 500,
  'minFeePaise': 9900,
  'maxFeePaise': 499900,
  'isFreeEvent': false,
  'baseFeePaise': 160000,
  'gstBp': 1800,
  'gstPaise': 28800,
  'totalPaise': 188800,
  'refundPolicy': {'beforeFirstRegistrationPercent': 100, 'afterRegistrationsPercent': 0, 'onAdminRejectionPercent': 100},
  'refundPolicyDocumentId': 9,
};

final messageJson = <String, dynamic>{
  'id': 101,
  'conversationId': 5,
  'sender': {'id': 42, 'displayName': 'Asha', 'isVerified': true},
  'kind': 'text',
  'body': 'See you at the ground',
  'imageUrl': null,
  'replyTo': null,
  'clientMsgId': 'c-1',
  'status': 'visible',
  'createdAt': '2026-03-04T10:00:00Z',
};

final eventJson = <String, dynamic>{
  'id': 9,
  'title': 'District U-16 trials',
  'eventType': 'trial',
  'competitionLevel': 'district',
  'entryFeePaise': 0,
  'maxSlots': 40,
  'registrationsCount': 10,
  'slotsLeft': 30,
  'status': 'published',
  'category': 'government',
  'eligibility': {
    'type': 'native',
    'scope': 'district',
    'district': {'id': 603, 'stateId': 33, 'name': 'Thanjavur'},
    'requireVerified': true,
    'summary': 'Native of Thanjavur district',
  },
  'myEligibility': {
    'eligible': false,
    'pending': false,
    'reason': 'identity_missing',
    'action': 'verify_native',
  },
  'distanceKm': 12.5,
};

void main() {
  test('Me round trip', () {
    final m = Me.fromJson(meJson);
    expect(m.isOnboarded, isTrue);
    expect(m.isOrganiser, isTrue);
    expect(m.pendingPolicies.single.version, 2);
    expect(m.createdAt, DateTime.utc(2026, 1, 2, 3, 4, 5));
    expectStable(meJson, Me.fromJson, (Me x) => x.toJson());
  });

  test('Me defaults for missing fields', () {
    final m = Me.fromJson({'id': 1, 'mobileMasked': 'x'});
    expect(m.onboardingStep, OnboardingStep.role);
    expect(m.status, 'active');
    expect(m.pendingPolicies, isEmpty);
  });

  test('ListingQuote round trip keeps integer paise', () {
    final q = ListingQuote.fromJson(quoteJson);
    expect(q.totalPaise, 188800);
    expect(q.totalPaise, isA<int>());
    expectStable(quoteJson, ListingQuote.fromJson, (ListingQuote x) => x.toJson());
  });

  test('Message round trip', () {
    final m = Message.fromJson(messageJson);
    expect(m.sender!.displayName, 'Asha');
    expect(m.isSent, isTrue);
    expect(m.isDeleted, isFalse);
    expectStable(messageJson, Message.fromJson, (Message x) => x.toJson());
    expect(m.markDeleted().isDeleted, isTrue);
    expect(m.markDeleted().body, isNull);
  });

  test('EventSummary parses eligibility, myEligibility and distance', () {
    final e = EventSummary.fromJson(eventJson);
    expect(e.category, EventCategory.government);
    expect(e.eligibility!.type, EligibilityType.native);
    expect(e.eligibility!.district!.name, 'Thanjavur');
    expect(e.myEligibility!.eligible, isFalse);
    expect(e.myEligibility!.action, EligibilityAction.verifyNative);
    expect(e.distanceKm, 12.5);
  });

  test('EventSummary without eligibility (feature off) defaults to private, no rule', () {
    final e = EventSummary.fromJson({'id': 1, 'title': 't', 'status': 'published'});
    expect(e.category, EventCategory.private);
    expect(e.eligibility, isNull);
    expect(e.myEligibility, isNull);
  });

  test('Registration flag and qualified label', () {
    final r = Registration.fromJson({
      'id': 1,
      'eventId': 9,
      'status': 'registered',
      'qualifiedVia': 'residence',
      'qualifiedDistrict': {'id': 603, 'stateId': 33, 'name': 'Thanjavur'},
      'eligibilityFlag': 'identity_expired',
      'eligibilityFlaggedAt': '2026-05-01T00:00:00Z',
      'qualifiedLabel': 'Residence: Thanjavur',
    });
    expect(r.isFlagged, isTrue);
    expect(r.qualifiedDistrict!.name, 'Thanjavur');
    expect(r.qualifiedLabel, 'Residence: Thanjavur');
    expect(eligibilityFlagText(r.eligibilityFlag), contains('expired'));

    final clean = Registration.fromJson({'id': 2, 'eventId': 9, 'status': 'registered'});
    expect(clean.isFlagged, isFalse);
  });
}

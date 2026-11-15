import '../../../core/json/json_utils.dart';
import 'geo_models.dart';

/// Wire values from docs/api.md (Events, `NOT_ELIGIBLE`).
abstract final class EligibilityType {
  static const open = 'open';
  static const native = 'native';
  static const residence = 'residence';
  static const either = 'either';
  static const all = [open, native, residence, either];
}

abstract final class EligibilityScope {
  static const district = 'district';
  static const state = 'state';
  static const all = [district, state];
}

abstract final class EventCategory {
  static const government = 'government';
  static const private = 'private';
  static const all = [government, private];
}

abstract final class EligibilityReason {
  static const identityMissing = 'identity_missing';
  static const identityPending = 'identity_pending';
  static const identityExpired = 'identity_expired';
  static const wrongLocation = 'wrong_location';
}

abstract final class EligibilityAction {
  static const none = 'none';
  static const verifyNative = 'verify_native';
  static const verifyResidence = 'verify_residence';
  static const renewResidence = 'renew_residence';
  static const waitForReview = 'wait_for_review';
}

/// `EligibilityRule` as returned on events (with names).
class EligibilityRule {
  const EligibilityRule({
    required this.type,
    this.scope,
    this.district,
    this.state,
    required this.requireVerified,
    this.summary,
  });

  final String type;
  final String? scope;
  final District? district;
  final GeoState? state;
  final bool requireVerified;
  final String? summary;

  bool get isOpen => type == EligibilityType.open;

  static const openRule = EligibilityRule(type: EligibilityType.open, requireVerified: true);

  factory EligibilityRule.fromJson(Map<String, dynamic> json) => EligibilityRule(
        type: asString(json['type'], EligibilityType.open),
        scope: asStringOrNull(json['scope']),
        district: District.maybe(json['district']),
        state: GeoState.maybe(json['state']),
        requireVerified: asBool(json['requireVerified'], true),
        summary: asStringOrNull(json['summary']),
      );

  static EligibilityRule? maybe(Object? v) {
    final m = asMapOrNull(v);
    return m == null ? null : EligibilityRule.fromJson(m);
  }

  Map<String, dynamic> toJson() => {
        'type': type,
        'scope': scope,
        'district': district?.toJson(),
        'state': state?.toJson(),
        'requireVerified': requireVerified,
        'summary': summary,
      };
}

/// `MyEligibility`: the caller's standing for one event.
class MyEligibility {
  const MyEligibility({
    required this.eligible,
    this.via,
    required this.pending,
    this.tag,
    this.reason,
    required this.action,
  });

  final bool eligible;
  final String? via;
  final bool pending;
  final String? tag;
  final String? reason;
  final String action;

  factory MyEligibility.fromJson(Map<String, dynamic> json) => MyEligibility(
        eligible: asBool(json['eligible'], true),
        via: asStringOrNull(json['via']),
        pending: asBool(json['pending']),
        tag: asStringOrNull(json['tag']),
        reason: asStringOrNull(json['reason']),
        action: asString(json['action'], EligibilityAction.none),
      );

  static MyEligibility? maybe(Object? v) {
    final m = asMapOrNull(v);
    return m == null ? null : MyEligibility.fromJson(m);
  }

  Map<String, dynamic> toJson() => {
        'eligible': eligible,
        'via': via,
        'pending': pending,
        'tag': tag,
        'reason': reason,
        'action': action,
      };
}

/// Eligibility part of `EventInput` (ids, not names).
class EligibilityInput {
  EligibilityInput({
    this.type = EligibilityType.open,
    this.scope,
    this.districtId,
    this.stateId,
    this.requireVerified = true,
  });

  String type;
  String? scope;
  int? districtId;
  int? stateId;
  bool requireVerified;

  factory EligibilityInput.fromRule(EligibilityRule? r) => EligibilityInput(
        type: r?.type ?? EligibilityType.open,
        scope: r?.scope,
        districtId: r?.district?.id,
        stateId: r?.state?.id ?? r?.district?.stateId,
        requireVerified: r?.requireVerified ?? true,
      );

  /// Open rules carry no scope or ids (else 422). District scope sends the
  /// district only; the server derives the state.
  Map<String, dynamic> toJson() {
    if (type == EligibilityType.open) {
      return {'type': type, 'scope': null, 'districtId': null, 'stateId': null, 'requireVerified': requireVerified};
    }
    final district = scope == EligibilityScope.district;
    return {
      'type': type,
      'scope': scope,
      'districtId': district ? districtId : null,
      'stateId': district ? null : stateId,
      'requireVerified': requireVerified,
    };
  }

  /// Query for `GET /events/eligibility/preview`.
  Map<String, Object?> toPreviewQuery(String category) {
    final j = toJson();
    return {
      'category': category,
      'type': j['type'],
      'scope': j['scope'],
      'districtId': j['districtId'],
      'stateId': j['stateId'],
      'requireVerified': requireVerified,
    };
  }
}

/// `EligibilityPreview`.
class EligibilityPreview {
  const EligibilityPreview({
    required this.valid,
    required this.fields,
    this.summary,
    this.suggestedType,
  });

  final bool valid;
  final Map<String, String> fields;
  final String? summary;
  final String? suggestedType;

  factory EligibilityPreview.fromJson(Map<String, dynamic> json) {
    final raw = asMapOrNull(json['fields']) ?? const <String, dynamic>{};
    return EligibilityPreview(
      valid: asBool(json['valid']),
      fields: {for (final e in raw.entries) if (e.value != null) e.key: '${e.value}'},
      summary: asStringOrNull(json['summary']),
      suggestedType: asStringOrNull(json['suggestedType']),
    );
  }
}

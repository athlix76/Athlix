import '../json/json_utils.dart';

/// An active policy version the user still has to accept.
class PendingPolicy {
  const PendingPolicy({
    required this.id,
    required this.docType,
    required this.version,
    required this.title,
  });

  final int id;
  final String docType;
  final int version;
  final String title;

  factory PendingPolicy.fromJson(Map<String, dynamic> json) => PendingPolicy(
        id: asInt(json['id']),
        docType: asString(json['docType']),
        version: asInt(json['version'], 1),
        title: asString(json['title']),
      );

  Map<String, dynamic> toJson() =>
      {'id': id, 'docType': docType, 'version': version, 'title': title};
}

/// Onboarding steps, in order (docs/api.md "Me, onboarding, verification").
abstract final class OnboardingStep {
  static const role = 'role';
  static const verification = 'verification';
  static const guardian = 'guardian';
  static const profile = 'profile';
  static const done = 'done';
}

/// `Me`: the signed-in user. `mobileMasked` is the only form of the mobile
/// number the app ever sees.
class Me {
  const Me({
    required this.id,
    required this.displayName,
    required this.mobileMasked,
    required this.primaryRole,
    required this.roles,
    required this.status,
    required this.onboardingStep,
    required this.verificationStatus,
    required this.isMinor,
    required this.guardianConsentStatus,
    required this.photoUrl,
    required this.storeAccountToken,
    required this.pendingPolicies,
    required this.createdAt,
  });

  final int id;
  final String? displayName;
  final String mobileMasked;
  final String? primaryRole;
  final List<String> roles;
  final String status;
  final String onboardingStep;
  final String verificationStatus;
  final bool isMinor;
  final String guardianConsentStatus;
  final String? photoUrl;
  final String? storeAccountToken;
  final List<PendingPolicy> pendingPolicies;
  final DateTime? createdAt;

  bool get isOnboarded => onboardingStep == OnboardingStep.done;
  bool get isVerified => verificationStatus == 'verified';
  bool get isSuspended => status == 'suspended';
  bool hasRole(String role) => roles.contains(role) || primaryRole == role;
  bool get isOrganiser => hasRole('organiser');
  bool get isCoach => hasRole('coach');

  factory Me.fromJson(Map<String, dynamic> json) => Me(
        id: asInt(json['id']),
        displayName: asStringOrNull(json['displayName']),
        mobileMasked: asString(json['mobileMasked']),
        primaryRole: asStringOrNull(json['primaryRole']),
        roles: asStringList(json['roles']),
        status: asString(json['status'], 'active'),
        onboardingStep: asString(json['onboardingStep'], OnboardingStep.role),
        verificationStatus: asString(json['verificationStatus'], 'unverified'),
        isMinor: asBool(json['isMinor']),
        guardianConsentStatus: asString(json['guardianConsentStatus'], 'not_required'),
        photoUrl: asStringOrNull(json['photoUrl']),
        storeAccountToken: asStringOrNull(json['storeAccountToken']),
        pendingPolicies: asList(json['pendingPolicies'], PendingPolicy.fromJson),
        createdAt: asDateTimeOrNull(json['createdAt']),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'displayName': displayName,
        'mobileMasked': mobileMasked,
        'primaryRole': primaryRole,
        'roles': roles,
        'status': status,
        'onboardingStep': onboardingStep,
        'verificationStatus': verificationStatus,
        'isMinor': isMinor,
        'guardianConsentStatus': guardianConsentStatus,
        'photoUrl': photoUrl,
        'storeAccountToken': storeAccountToken,
        'pendingPolicies': pendingPolicies.map((p) => p.toJson()).toList(),
        'createdAt': formatRfc3339OrNull(createdAt),
      };

  Me copyWith({String? onboardingStep, String? status}) => Me(
        id: id,
        displayName: displayName,
        mobileMasked: mobileMasked,
        primaryRole: primaryRole,
        roles: roles,
        status: status ?? this.status,
        onboardingStep: onboardingStep ?? this.onboardingStep,
        verificationStatus: verificationStatus,
        isMinor: isMinor,
        guardianConsentStatus: guardianConsentStatus,
        photoUrl: photoUrl,
        storeAccountToken: storeAccountToken,
        pendingPolicies: pendingPolicies,
        createdAt: createdAt,
      );
}

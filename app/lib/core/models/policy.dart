import '../json/json_utils.dart';

/// Policy reference from `Bootstrap.policies`.
class PolicyRef {
  const PolicyRef({
    required this.id,
    required this.docType,
    required this.version,
    required this.title,
    required this.requiresAcceptance,
  });

  final int id;
  final String docType;
  final int version;
  final String title;
  final bool requiresAcceptance;

  factory PolicyRef.fromJson(Map<String, dynamic> json) => PolicyRef(
        id: asInt(json['id']),
        docType: asString(json['docType']),
        version: asInt(json['version'], 1),
        title: asString(json['title']),
        requiresAcceptance: asBool(json['requiresAcceptance']),
      );
}

/// Known `docType` values.
abstract final class PolicyDocType {
  static const terms = 'terms';
  static const privacy = 'privacy';
  static const listingRefund = 'listing_refund';
  static const guardianConsent = 'guardian_consent';
  static const communityGuidelines = 'community_guidelines';
  static const organiserTerms = 'organiser_terms';
  static const entryFeeDisclaimer = 'entry_fee_disclaimer';
  static const locationIdentityNotice = 'location_identity_notice';
}

/// `PolicyDocument` (`GET /policies/:docType`).
class PolicyDocument {
  const PolicyDocument({
    required this.id,
    required this.docType,
    required this.version,
    required this.title,
    required this.contentMd,
    required this.requiresAcceptance,
    required this.effectiveFrom,
  });

  final int id;
  final String docType;
  final int version;
  final String title;
  final String contentMd;
  final bool requiresAcceptance;
  final DateTime? effectiveFrom;

  factory PolicyDocument.fromJson(Map<String, dynamic> json) => PolicyDocument(
        id: asInt(json['id']),
        docType: asString(json['docType']),
        version: asInt(json['version'], 1),
        title: asString(json['title']),
        contentMd: asString(json['contentMd']),
        requiresAcceptance: asBool(json['requiresAcceptance']),
        effectiveFrom: asDateTimeOrNull(json['effectiveFrom']),
      );
}

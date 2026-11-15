import '../../../core/json/json_utils.dart';

/// `OrganiserVerification.status`.
abstract final class OrganiserStatus {
  static const notStarted = 'not_started';
  static const draft = 'draft';
  static const submitted = 'submitted';
  static const approved = 'approved';
  static const rejected = 'rejected';

  /// Statuses in which `PUT` / submit are allowed.
  static const editable = {notStarted, draft, rejected};
}

/// `orgType` values.
abstract final class OrgType {
  static const values = [
    'individual',
    'club',
    'academy',
    'school',
    'college',
    'association',
    'company',
    'other',
  ];
}

/// `inputType` of a step. Unknown types are shown read-only.
abstract final class StepInputType {
  static const text = 'text';
  static const document = 'document';
  static const checkbox = 'checkbox';
}

/// A stored document: the app only ever sees the upload id and a signed URL.
class StepDocument {
  const StepDocument({required this.uploadId, required this.url});

  final int uploadId;
  final String? url;

  static StepDocument? maybe(Object? json) {
    final m = asMapOrNull(json);
    if (m == null) return null;
    return StepDocument(uploadId: asInt(m['uploadId']), url: asStringOrNull(m['url']));
  }
}

class StepSubmission {
  const StepSubmission({
    required this.valueText,
    required this.document,
    required this.checked,
    required this.status,
    required this.reviewNote,
  });

  final String? valueText;
  final StepDocument? document;
  final bool checked;
  final String? status;
  final String? reviewNote;

  static StepSubmission? maybe(Object? json) {
    final m = asMapOrNull(json);
    if (m == null) return null;
    return StepSubmission(
      valueText: asStringOrNull(m['valueText']),
      document: StepDocument.maybe(m['document']),
      checked: asBool(m['checked']),
      status: asStringOrNull(m['status']),
      reviewNote: asStringOrNull(m['reviewNote']),
    );
  }
}

/// One server-defined step (`organiser_verification_steps`).
class VerificationStep {
  const VerificationStep({
    required this.stepId,
    required this.stepCode,
    required this.title,
    required this.description,
    required this.inputType,
    required this.isRequired,
    required this.submission,
  });

  final int stepId;
  final String stepCode;
  final String title;
  final String? description;
  final String inputType;
  final bool isRequired;
  final StepSubmission? submission;

  factory VerificationStep.fromJson(Map<String, dynamic> json) => VerificationStep(
        stepId: asInt(json['stepId']),
        stepCode: asString(json['stepCode']),
        title: asString(json['title']),
        description: asStringOrNull(json['description']),
        inputType: asString(json['inputType'], StepInputType.text),
        isRequired: asBool(json['isRequired']),
        submission: StepSubmission.maybe(json['submission']),
      );
}

/// `OrganiserVerification`.
class OrganiserVerification {
  const OrganiserVerification({
    required this.status,
    required this.orgName,
    required this.orgType,
    required this.reviewNote,
    required this.submittedAt,
    required this.reviewedAt,
    required this.steps,
  });

  final String status;
  final String? orgName;
  final String? orgType;
  final String? reviewNote;
  final DateTime? submittedAt;
  final DateTime? reviewedAt;
  final List<VerificationStep> steps;

  bool get isEditable => OrganiserStatus.editable.contains(status);

  factory OrganiserVerification.fromJson(Map<String, dynamic> json) => OrganiserVerification(
        status: asString(json['status'], OrganiserStatus.notStarted),
        orgName: asStringOrNull(json['orgName']),
        orgType: asStringOrNull(json['orgType']),
        reviewNote: asStringOrNull(json['reviewNote']),
        submittedAt: asDateTimeOrNull(json['submittedAt']),
        reviewedAt: asDateTimeOrNull(json['reviewedAt']),
        steps: asList(json['steps'], VerificationStep.fromJson),
      );
}

/// One entry of `steps` in `PUT /organiser/verification`.
class StepInput {
  const StepInput({required this.stepId, this.valueText, this.documentUploadId, this.checked});

  final int stepId;
  final String? valueText;
  final int? documentUploadId;
  final bool? checked;

  Map<String, dynamic> toJson() => {
        'stepId': stepId,
        if (valueText != null) 'valueText': valueText,
        if (documentUploadId != null) 'documentUploadId': documentUploadId,
        if (checked != null) 'checked': checked,
      };
}

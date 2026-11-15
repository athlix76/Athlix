import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/models/policy.dart';
import '../data/policy_api.dart';

/// Active policy document by `docType`.
final policyDocumentProvider = FutureProvider.autoDispose.family<PolicyDocument, String>(
  (ref, docType) => ref.watch(policyApiProvider).get(docType),
);

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/policies_api.dart';

final policiesApiProvider = Provider<PoliciesApi>((ref) => PoliciesApi(ref.watch(apiClientProvider)));

/// All versions, optionally of one type (null = all types).
final policiesProvider = FutureProvider.autoDispose.family<List<PolicyDocument>, String?>(
  (ref, docType) => ref.watch(policiesApiProvider).list(docType: docType),
);

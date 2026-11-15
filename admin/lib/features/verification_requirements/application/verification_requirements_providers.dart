import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/verification_requirements_api.dart';

final verificationRequirementsApiProvider =
    Provider<VerificationRequirementsApi>((ref) => VerificationRequirementsApi(ref.watch(apiClientProvider)));

final verificationRequirementsProvider = FutureProvider.autoDispose<List<VerificationRequirement>>(
  (ref) => ref.watch(verificationRequirementsApiProvider).list(),
);

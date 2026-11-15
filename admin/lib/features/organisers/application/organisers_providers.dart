import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/organisers_api.dart';

final organisersApiProvider = Provider<OrganisersApi>((ref) => OrganisersApi(ref.watch(apiClientProvider)));

/// Re-fetched on every open so document URLs are freshly signed.
final organiserDetailProvider = FutureProvider.autoDispose.family<OrganiserApplication, int>(
  (ref, userId) => ref.watch(organisersApiProvider).detail(userId),
);

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/location_review_api.dart';

final locationReviewApiProvider = Provider<LocationReviewApi>((ref) => LocationReviewApi(ref.watch(apiClientProvider)));

/// autoDispose: leaving the screen drops the signed proof URL from memory.
final locationDetailProvider = FutureProvider.autoDispose.family<LocationDetail, int>(
  (ref, id) => ref.watch(locationReviewApiProvider).detail(id),
);

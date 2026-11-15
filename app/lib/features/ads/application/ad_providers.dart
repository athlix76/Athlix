import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../data/ad_api.dart';
import '../data/ad_models.dart';

/// Ads for one slot. Errors are swallowed into `[]`: an ad must never break a screen.
final adsProvider = FutureProvider.autoDispose.family<List<Ad>, AdQuery>((ref, q) async {
  try {
    return await ref.watch(adApiProvider).list(q);
  } catch (_) {
    return const <Ad>[];
  }
});

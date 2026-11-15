import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/ads_api.dart';

final adsApiProvider = Provider<AdsApi>((ref) => AdsApi(ref.watch(apiClientProvider)));

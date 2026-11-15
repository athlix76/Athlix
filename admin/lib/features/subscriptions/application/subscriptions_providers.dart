import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/subscriptions_api.dart';

final subscriptionsApiProvider = Provider<SubscriptionsApi>((ref) => SubscriptionsApi(ref.watch(apiClientProvider)));

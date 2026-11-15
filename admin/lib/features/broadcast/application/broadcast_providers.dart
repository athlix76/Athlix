import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/broadcast_api.dart';

final broadcastApiProvider = Provider<BroadcastApi>((ref) => BroadcastApi(ref.watch(apiClientProvider)));

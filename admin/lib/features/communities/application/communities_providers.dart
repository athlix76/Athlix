import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/communities_api.dart';

final communitiesApiProvider = Provider<CommunitiesApi>((ref) => CommunitiesApi(ref.watch(apiClientProvider)));

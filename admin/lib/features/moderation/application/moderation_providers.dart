import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/moderation_api.dart';

final moderationApiProvider = Provider<ModerationApi>((ref) => ModerationApi(ref.watch(apiClientProvider)));

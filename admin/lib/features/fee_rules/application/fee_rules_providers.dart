import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/fee_rules_api.dart';

final feeRulesApiProvider = Provider<FeeRulesApi>((ref) => FeeRulesApi(ref.watch(apiClientProvider)));

final feeRulesProvider = FutureProvider.autoDispose<List<FeeRule>>((ref) => ref.watch(feeRulesApiProvider).list());

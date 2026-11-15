import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/reports_api.dart';

final reportsApiProvider = Provider<ReportsApi>((ref) => ReportsApi(ref.watch(apiClientProvider)));

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/audit_api.dart';

final auditApiProvider = Provider<AuditApi>((ref) => AuditApi(ref.watch(apiClientProvider)));

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/auth/admin_me.dart';
import '../../../core/providers.dart';
import '../data/admins_api.dart';

final adminsApiProvider = Provider<AdminsApi>((ref) => AdminsApi(ref.watch(apiClientProvider)));

final adminUsersProvider = FutureProvider.autoDispose<List<AdminMe>>((ref) => ref.watch(adminsApiProvider).admins());

final adminRolesProvider = FutureProvider.autoDispose<List<AdminRole>>((ref) => ref.watch(adminsApiProvider).roles());

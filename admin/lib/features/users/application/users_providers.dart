import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/users_api.dart';

final usersApiProvider = Provider<UsersApi>((ref) => UsersApi(ref.watch(apiClientProvider)));

final userDetailProvider = FutureProvider.autoDispose.family<AdminUserDetail, int>(
  (ref, id) => ref.watch(usersApiProvider).detail(id),
);

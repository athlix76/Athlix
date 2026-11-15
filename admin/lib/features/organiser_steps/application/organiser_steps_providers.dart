import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/organiser_steps_api.dart';

final organiserStepsApiProvider = Provider<OrganiserStepsApi>((ref) => OrganiserStepsApi(ref.watch(apiClientProvider)));

final organiserStepsProvider = FutureProvider.autoDispose<List<OrganiserStep>>((ref) => ref.watch(organiserStepsApiProvider).list());

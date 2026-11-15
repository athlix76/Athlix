import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/providers.dart';
import '../data/events_api.dart';

final eventsApiProvider = Provider<EventsApi>((ref) => EventsApi(ref.watch(apiClientProvider)));

final eventDetailProvider = FutureProvider.autoDispose.family<AdminEvent, int>(
  (ref, id) => ref.watch(eventsApiProvider).detail(id),
);

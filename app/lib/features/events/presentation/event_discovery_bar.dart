import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/location/device_location.dart';
import '../../../core/widgets/error_view.dart';
import '../application/event_providers.dart';

/// Radius choices offered by the app; capped by `event.max_search_radius_km`.
const radiusChoicesKm = [10, 25, 50, 100];

List<int> allowedRadii(int? maxKm) =>
    maxKm == null ? radiusChoicesKm : radiusChoicesKm.where((r) => r <= maxKm).toList();

/// "Show all events" toggle and "Near me" radius chips above the events list.
/// The device location is read on demand, kept in [eventFiltersProvider]
/// (memory only) and never stored or logged.
class EventDiscoveryBar extends ConsumerWidget {
  const EventDiscoveryBar({super.key});

  Future<void> _pickRadius(BuildContext context, WidgetRef ref, int km) async {
    final notifier = ref.read(eventFiltersProvider.notifier);
    final f = notifier.state;
    if (f.radiusKm == km) {
      notifier.state = f.copyWith(near: () => null, radiusKm: () => null);
      return;
    }
    var near = f.near;
    if (near == null) {
      try {
        near = await ref.read(deviceLocationProvider).current();
      } catch (_) {
        near = null;
      }
      if (near == null) {
        ref.read(locationAccessProvider.notifier).state = LocationAccess.denied;
        if (context.mounted) showInfoSnack(context, 'Location is off, so distance filters are hidden.');
        return;
      }
      ref.read(locationAccessProvider.notifier).state = LocationAccess.granted;
    }
    notifier.state = notifier.state.copyWith(near: () => near, radiusKm: () => km);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final filters = ref.watch(eventFiltersProvider);
    final config = ref.watch(appConfigProvider);
    final access = ref.watch(locationAccessProvider);
    final radii = allowedRadii(config.eventMaxSearchRadiusKm);
    final showDistance = access != LocationAccess.denied && radii.isNotEmpty;

    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: Row(
        children: [
          if (config.locationEnabled)
            Padding(
              padding: const EdgeInsets.only(right: 8),
              child: FilterChip(
                label: const Text('Show all events'),
                selected: filters.showAll,
                onSelected: (v) =>
                    ref.read(eventFiltersProvider.notifier).state = filters.copyWith(showAll: v),
              ),
            ),
          if (showDistance) ...[
            const Padding(
              padding: EdgeInsets.only(right: 4),
              child: Icon(Icons.near_me_outlined, size: 18),
            ),
            for (final km in radii)
              Padding(
                padding: const EdgeInsets.only(right: 6),
                child: ChoiceChip(
                  label: Text('$km km'),
                  selected: filters.radiusKm == km,
                  onSelected: (_) => _pickRadius(context, ref, km),
                ),
              ),
          ],
        ],
      ),
    );
  }
}

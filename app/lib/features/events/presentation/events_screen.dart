import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../ads/presentation/ad_slot.dart';
import '../application/event_providers.dart';
import '../data/event_api.dart';
import '../data/event_models.dart';
import 'event_card.dart';
import 'event_discovery_bar.dart';
import 'event_filters_sheet.dart';

/// Events tab: search, filters, list of published events.
class EventsScreen extends ConsumerStatefulWidget {
  const EventsScreen({super.key});

  @override
  ConsumerState<EventsScreen> createState() => _EventsScreenState();
}

class _EventsScreenState extends ConsumerState<EventsScreen> {
  late final _search = TextEditingController(text: ref.read(eventFiltersProvider).q ?? '');

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  void _setFilters(EventFilters f) => ref.read(eventFiltersProvider.notifier).state = f;

  @override
  Widget build(BuildContext context) {
    final filters = ref.watch(eventFiltersProvider);
    final me = ref.watch(meProvider);
    final api = ref.watch(eventApiProvider);
    return Scaffold(
      appBar: AppBar(
        title: const Text('Events'),
        actions: [
          IconButton(
            tooltip: 'My registrations',
            icon: const Icon(Icons.confirmation_number_outlined),
            onPressed: () => context.push(Routes.myRegistrations),
          ),
          if (me?.isOrganiser ?? false)
            IconButton(
              tooltip: 'My events',
              icon: const Icon(Icons.event_note_outlined),
              onPressed: () => context.push(Routes.myEvents),
            ),
        ],
      ),
      floatingActionButton: (me?.isOrganiser ?? false)
          ? FloatingActionButton.extended(
              onPressed: () => context.push(Routes.eventNew),
              icon: const Icon(Icons.add),
              label: const Text('Create event'),
            )
          : null,
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
            child: Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _search,
                    textInputAction: TextInputAction.search,
                    decoration: const InputDecoration(
                      hintText: 'Search events',
                      prefixIcon: Icon(Icons.search),
                      isDense: true,
                    ),
                    onSubmitted: (v) => _setFilters(filters.withQuery(v.trim().isEmpty ? null : v.trim())),
                  ),
                ),
                const SizedBox(width: 8),
                Badge(
                  isLabelVisible: filters.activeCount > 0,
                  label: Text('${filters.activeCount}'),
                  child: IconButton.outlined(
                    tooltip: 'Filters',
                    icon: const Icon(Icons.tune),
                    onPressed: () async {
                      final f = await showEventFiltersSheet(context, filters);
                      if (f != null) _setFilters(f);
                    },
                  ),
                ),
              ],
            ),
          ),
          const EventDiscoveryBar(),
          Expanded(
            child: CursorListView<EventSummary>(
              key: ValueKey(filters),
              fetch: (cursor) => api.list(filters, cursor: cursor),
              header: AdSlot(placement: AdPlacement.eventsList, sportId: filters.sportId, state: filters.state),
              emptyText: filters.showAll
                  ? 'No events match. Try other filters.'
                  : 'No events you can join match. Try other filters or "Show all events".',
              itemBuilder: (context, e, _) => EventCard(event: e),
            ),
          ),
        ],
      ),
    );
  }
}

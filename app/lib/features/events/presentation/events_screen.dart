import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/page_hero.dart';
import '../../../core/widgets/section.dart';
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
    final organiser = me?.isOrganiser ?? false;
    return Scaffold(
      backgroundColor: Colors.transparent,
      body: Column(
        children: [
          PageHero(
            eyebrow: 'Tournaments',
            title: 'Find your next challenge.',
            subtitle: 'Local grounds. Big moments. Your game starts here.',
            bottom: Wrap(
              spacing: 10,
              runSpacing: 10,
              children: [
                if (organiser)
                  HeroButton(
                    icon: Icons.add,
                    label: 'Host an event',
                    primary: true,
                    onPressed: () => context.push(Routes.eventNew),
                  ),
                if (organiser)
                  HeroButton(
                    icon: Icons.event_note_outlined,
                    label: 'My events',
                    onPressed: () => context.push(Routes.myEvents),
                  ),
                HeroButton(
                  icon: Icons.confirmation_number_outlined,
                  label: 'My tournaments',
                  onPressed: () => context.push(Routes.myRegistrations),
                ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 10),
            child: Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _search,
                    textInputAction: TextInputAction.search,
                    decoration: searchFieldDecoration('Search tournaments, sports or cities'),
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
                    style: IconButton.styleFrom(
                      backgroundColor: AppTheme.surface,
                      side: const BorderSide(color: AppTheme.line),
                      fixedSize: const Size(50, 50),
                      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
                    ),
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
              header: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                mainAxisSize: MainAxisSize.min,
                children: [
                  const SectionHeader('Upcoming tournaments'),
                  AdSlot(placement: AdPlacement.eventsList, sportId: filters.sportId, state: filters.state),
                ],
              ),
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

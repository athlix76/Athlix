import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/router/routes.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/section.dart';
import '../../../core/widgets/user_avatar.dart';
import '../../../core/widgets/user_tile.dart';
import '../../ads/presentation/ad_slot.dart';
import '../../events/presentation/event_card.dart';
import '../application/search_providers.dart';
import '../data/search_models.dart';

/// Search everything: people, events and communities (`GET /search`, max 5 each).
class SearchScreen extends ConsumerStatefulWidget {
  const SearchScreen({super.key});

  @override
  ConsumerState<SearchScreen> createState() => _SearchScreenState();
}

class _SearchScreenState extends ConsumerState<SearchScreen> {
  final _ctrl = TextEditingController();
  Timer? _debounce;
  String _q = '';

  @override
  void dispose() {
    _debounce?.cancel();
    _ctrl.dispose();
    super.dispose();
  }

  void _onChanged(String value) {
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 350), () {
      if (mounted) setState(() => _q = value.trim());
    });
  }

  void _openPeople() {
    final filters = ref.read(peopleFiltersProvider);
    ref.read(peopleFiltersProvider.notifier).state = filters.withQuery(_q.isEmpty ? null : _q);
    context.push(Routes.searchPeople);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: TextField(
          controller: _ctrl,
          autofocus: true,
          textInputAction: TextInputAction.search,
          decoration: const InputDecoration(
            hintText: 'Search',
            border: InputBorder.none,
          ),
          onChanged: _onChanged,
          onSubmitted: (v) => setState(() => _q = v.trim()),
        ),
        actions: [
          IconButton(
            tooltip: 'Find people',
            icon: const Icon(Icons.person_search_outlined),
            onPressed: _openPeople,
          ),
        ],
      ),
      body: _q.length < kMinSearchLength
          ? EmptyView(
              text: 'Type at least $kMinSearchLength letters to search people, events and communities.',
              icon: Icons.search,
              action: OutlinedButton.icon(
                onPressed: _openPeople,
                icon: const Icon(Icons.tune),
                label: const Text('Find people with filters'),
              ),
            )
          : AsyncValueView<SearchResults>(
              value: ref.watch(searchResultsProvider(_q)),
              onRetry: () => ref.invalidate(searchResultsProvider(_q)),
              data: (r) => r.isEmpty ? EmptyView(text: 'No results for "$_q".') : _Results(results: r, onSeeAllPeople: _openPeople),
            ),
    );
  }
}

class _Results extends StatelessWidget {
  const _Results({required this.results, required this.onSeeAllPeople});

  final SearchResults results;
  final VoidCallback onSeeAllPeople;

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.only(bottom: 24),
      children: [
        if (results.people.isNotEmpty) ...[
          SectionHeader(
            'People',
            trailing: TextButton(onPressed: onSeeAllPeople, child: const Text('See all')),
          ),
          for (final u in results.people) UserTile(user: u),
        ],
        if (results.events.isNotEmpty) ...[
          const SectionHeader('Events'),
          for (final e in results.events) EventCard(event: e),
        ],
        if (results.communities.isNotEmpty) ...[
          const SectionHeader('Communities'),
          for (final c in results.communities)
            ListTile(
              leading: UserAvatar(name: c.title, url: c.avatarUrl),
              title: Text(c.title),
              subtitle: Text(c.subtitle),
              onTap: () => context.push(Routes.community(c.id)),
            ),
        ],
        const AdSlot(placement: AdPlacement.search),
      ],
    );
  }
}

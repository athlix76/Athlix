import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/router/routes.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/page_hero.dart';
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

  /// Which result group to show: all, people, events or communities.
  int _kind = 0;
  static const _kinds = ['All', 'People', 'Events', 'Communities'];

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
      backgroundColor: Colors.transparent,
      body: Column(
        children: [
          const PageHero(
            eyebrow: 'The sporting community',
            title: 'Find your people.',
            subtitle: 'A new teammate. A great coach. Your next game.',
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 12),
            child: TextField(
              controller: _ctrl,
              textInputAction: TextInputAction.search,
              decoration: searchFieldDecoration(
                'Search people, sports or places',
                suffix: IconButton(
                  tooltip: 'Find people with filters',
                  icon: const Icon(Icons.tune),
                  onPressed: _openPeople,
                ),
              ),
              onChanged: _onChanged,
              onSubmitted: (v) => setState(() => _q = v.trim()),
            ),
          ),
          PillChips(labels: _kinds, selected: _kind, onSelected: (i) => setState(() => _kind = i)),
          const SizedBox(height: 4),
          Expanded(
            child: _q.length < kMinSearchLength
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
                    data: (r) => r.isEmpty
                        ? EmptyView(text: 'No results for "$_q".')
                        : _Results(results: r, kind: _kind, onSeeAllPeople: _openPeople),
                  ),
          ),
        ],
      ),
    );
  }
}

class _Results extends StatelessWidget {
  const _Results({required this.results, required this.kind, required this.onSeeAllPeople});

  final SearchResults results;

  /// 0 all, 1 people, 2 events, 3 communities.
  final int kind;
  final VoidCallback onSeeAllPeople;

  @override
  Widget build(BuildContext context) {
    final people = kind == 0 || kind == 1 ? results.people : const [];
    final events = kind == 0 || kind == 2 ? results.events : const [];
    final communities = kind == 0 || kind == 3 ? results.communities : const [];
    final count = people.length + events.length + communities.length;
    return ListView(
      padding: const EdgeInsets.only(bottom: 24),
      children: [
        if (count == 0)
          const Padding(
            padding: EdgeInsets.only(top: 40),
            child: EmptyView(text: 'Nothing in this group. Try "All".'),
          ),
        if (people.isNotEmpty) ...[
          SectionHeader(
            'People to follow',
            trailing: TextButton(onPressed: onSeeAllPeople, child: const Text('See all')),
          ),
          _GroupCard(children: [for (final u in people) UserTile(user: u)]),
        ],
        if (events.isNotEmpty) ...[
          const SectionHeader('Get in the game'),
          for (final e in events) EventCard(event: e),
        ],
        if (communities.isNotEmpty) ...[
          const SectionHeader('Communities'),
          _GroupCard(
            children: [
              for (final c in communities)
                ListTile(
                  leading: UserAvatar(name: c.title, url: c.avatarUrl),
                  title: Text(c.title),
                  subtitle: Text(c.subtitle),
                  trailing: const Icon(Icons.chevron_right),
                  onTap: () => context.push(Routes.community(c.id)),
                ),
            ],
          ),
        ],
        const AdSlot(placement: AdPlacement.search),
      ],
    );
  }
}

/// White rounded card holding a list, rows separated by thin lines (template `.card`).
class _GroupCard extends StatelessWidget {
  const _GroupCard({required this.children});

  final List<Widget> children;

  @override
  Widget build(BuildContext context) => Card(
        clipBehavior: Clip.antiAlias,
        child: Column(
          children: [
            for (var i = 0; i < children.length; i++) ...[
              if (i > 0) const Divider(indent: 16, endIndent: 16),
              Padding(padding: const EdgeInsets.symmetric(vertical: 4), child: children[i]),
            ],
          ],
        ),
      );
}

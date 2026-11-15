import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/router/routes.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/sport_picker.dart';
import '../application/community_providers.dart';
import '../data/community_api.dart';
import '../data/community_models.dart';
import 'community_tile.dart';

/// Discover public communities (search + sport/state filters) and "Mine".
class CommunitiesScreen extends ConsumerStatefulWidget {
  const CommunitiesScreen({super.key});

  @override
  ConsumerState<CommunitiesScreen> createState() => _CommunitiesScreenState();
}

class _CommunitiesScreenState extends ConsumerState<CommunitiesScreen> {
  late final _q = TextEditingController(text: ref.read(communityFiltersProvider).q);
  late final _state = TextEditingController(text: ref.read(communityFiltersProvider).state);
  final _mine = CursorListController<Community>();
  Timer? _debounce;

  @override
  void dispose() {
    _q.dispose();
    _state.dispose();
    _debounce?.cancel();
    super.dispose();
  }

  void _setFilters(CommunityFilters f) => ref.read(communityFiltersProvider.notifier).state = f;

  void _onText() {
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 400), () {
      final f = ref.read(communityFiltersProvider);
      _setFilters((q: _q.text.trim(), sportId: f.sportId, state: _state.text.trim()));
    });
  }

  Future<void> _open(Community c) async {
    await context.push(Routes.community(c.id));
    if (mounted) unawaited(_mine.refresh());
  }

  @override
  Widget build(BuildContext context) {
    final f = ref.watch(communityFiltersProvider);
    final api = ref.watch(communityApiProvider);
    return DefaultTabController(
      length: 2,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Communities'),
          bottom: const TabBar(tabs: [Tab(text: 'Discover'), Tab(text: 'Mine')]),
        ),
        floatingActionButton: FloatingActionButton.extended(
          onPressed: () async {
            await context.push(Routes.communityNew);
            if (mounted) unawaited(_mine.refresh());
          },
          icon: const Icon(Icons.add),
          label: const Text('Create'),
        ),
        body: TabBarView(
          children: [
            Column(
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 12, 16, 4),
                  child: TextField(
                    controller: _q,
                    onChanged: (_) => _onText(),
                    textInputAction: TextInputAction.search,
                    decoration: const InputDecoration(prefixIcon: Icon(Icons.search), hintText: 'Search communities'),
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
                  child: Row(
                    children: [
                      Expanded(
                        child: SportDropdown(
                          value: f.sportId,
                          allowAny: true,
                          onChanged: (v) => _setFilters((q: f.q, sportId: v, state: f.state)),
                        ),
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: TextField(
                          controller: _state,
                          onChanged: (_) => _onText(),
                          decoration: const InputDecoration(labelText: 'State'),
                        ),
                      ),
                    ],
                  ),
                ),
                Expanded(
                  child: CursorListView<Community>(
                    key: ValueKey(f),
                    fetch: (cursor) => api.list(
                      q: f.q.isEmpty ? null : f.q,
                      sportId: f.sportId,
                      state: f.state.isEmpty ? null : f.state,
                      cursor: cursor,
                    ),
                    emptyText: 'No communities match. Create one!',
                    itemBuilder: (context, c, _) => CommunityTile(community: c, onTap: () => _open(c)),
                  ),
                ),
              ],
            ),
            RefreshIndicator(
              onRefresh: _mine.refresh,
              child: CursorListView<Community>(
                controller: _mine,
                fetch: (cursor) => api.mine(cursor: cursor),
                emptyText: "You haven't joined any communities yet.",
                itemBuilder: (context, c, _) => CommunityTile(community: c, onTap: () => _open(c)),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

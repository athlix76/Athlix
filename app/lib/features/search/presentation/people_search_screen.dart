import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/sport_picker.dart';
import '../../../core/widgets/user_tile.dart';
import '../../onboarding/data/onboarding_api.dart' show UserRole;
import '../application/search_providers.dart';
import '../data/search_api.dart';
import '../data/search_models.dart';

/// Find people with filters (`GET /search/people`). Advanced filters need a premium
/// benefit; without it the list shows the plan-required view returned by the server.
class PeopleSearchScreen extends ConsumerStatefulWidget {
  const PeopleSearchScreen({super.key});

  @override
  ConsumerState<PeopleSearchScreen> createState() => _PeopleSearchScreenState();
}

class _PeopleSearchScreenState extends ConsumerState<PeopleSearchScreen> {
  late final _ctrl = TextEditingController(text: ref.read(peopleFiltersProvider).q ?? '');
  Timer? _debounce;

  @override
  void dispose() {
    _debounce?.cancel();
    _ctrl.dispose();
    super.dispose();
  }

  void _setFilters(PeopleFilters f) => ref.read(peopleFiltersProvider.notifier).state = f;

  void _onChanged(String value) {
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 400), () {
      if (!mounted) return;
      final v = value.trim();
      _setFilters(ref.read(peopleFiltersProvider).withQuery(v.isEmpty ? null : v));
    });
  }

  Future<void> _openFilters() async {
    final result = await showModalBottomSheet<PeopleFilters>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (_) => _PeopleFiltersSheet(initial: ref.read(peopleFiltersProvider)),
    );
    if (result != null) _setFilters(result.withQuery(ref.read(peopleFiltersProvider).q));
  }

  @override
  Widget build(BuildContext context) {
    final filters = ref.watch(peopleFiltersProvider);
    final api = ref.watch(searchApiProvider);
    final limit = ref.watch(appConfigProvider).searchPageSize;
    final active = filters.activeCount;
    return Scaffold(
      appBar: AppBar(
        title: TextField(
          controller: _ctrl,
          textInputAction: TextInputAction.search,
          decoration: const InputDecoration(hintText: 'Search people', border: InputBorder.none),
          onChanged: _onChanged,
        ),
        actions: [
          IconButton(
            tooltip: 'Filters',
            onPressed: _openFilters,
            icon: Badge(
              isLabelVisible: active > 0,
              label: Text('$active'),
              child: const Icon(Icons.tune),
            ),
          ),
        ],
      ),
      body: CursorListView<PersonResult>(
        key: ValueKey(filters.cacheKey),
        fetch: (cursor) => api.people(filters, cursor: cursor, limit: limit),
        emptyText: 'No people match these filters.',
        separated: true,
        itemBuilder: (context, p, _) => UserTile(
          user: p.user,
          subtitle: [
            if (p.user.subtitle.isNotEmpty) p.user.subtitle,
            if (p.bio != null && p.bio!.isNotEmpty) p.bio!,
          ].join('\n'),
          trailing: Text('${p.followersCount}', style: Theme.of(context).textTheme.labelMedium),
        ),
      ),
    );
  }
}

class _PeopleFiltersSheet extends StatefulWidget {
  const _PeopleFiltersSheet({required this.initial});

  final PeopleFilters initial;

  @override
  State<_PeopleFiltersSheet> createState() => _PeopleFiltersSheetState();
}

class _PeopleFiltersSheetState extends State<_PeopleFiltersSheet> {
  late String? _role = widget.initial.role;
  late int? _sportId = widget.initial.sportId;
  late int? _positionId = widget.initial.positionId;
  late final _state = TextEditingController(text: widget.initial.state ?? '');
  late final _district = TextEditingController(text: widget.initial.district ?? '');
  late final _city = TextEditingController(text: widget.initial.city ?? '');
  late final _ageMin = TextEditingController(text: widget.initial.ageMin?.toString() ?? '');
  late final _ageMax = TextEditingController(text: widget.initial.ageMax?.toString() ?? '');
  late String? _level = widget.initial.achievementLevel;

  @override
  void dispose() {
    for (final c in [_state, _district, _city, _ageMin, _ageMax]) {
      c.dispose();
    }
    super.dispose();
  }

  String? _text(TextEditingController c) {
    final v = c.text.trim();
    return v.isEmpty ? null : v;
  }

  void _apply() => Navigator.of(context).pop(PeopleFilters(
        role: _role,
        sportId: _sportId,
        positionId: _positionId,
        state: _text(_state),
        district: _text(_district),
        city: _text(_city),
        ageMin: int.tryParse(_ageMin.text.trim()),
        ageMax: int.tryParse(_ageMax.text.trim()),
        achievementLevel: _level,
      ));

  @override
  Widget build(BuildContext context) {
    const gap = SizedBox(height: 12);
    final bottom = MediaQuery.of(context).viewInsets.bottom;
    return Padding(
      padding: EdgeInsets.fromLTRB(16, 0, 16, 16 + bottom),
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            Text('Filters', style: Theme.of(context).textTheme.titleLarge),
            gap,
            EnumDropdown(
              label: 'Role',
              values: UserRole.all,
              value: _role,
              allowAny: true,
              onChanged: (v) => setState(() => _role = v),
            ),
            gap,
            SportDropdown(
              value: _sportId,
              allowAny: true,
              onChanged: (v) => setState(() {
                _sportId = v;
                _positionId = null;
              }),
            ),
            gap,
            PositionDropdown(
              sportId: _sportId,
              value: _positionId,
              role: _role,
              allowAny: true,
              onChanged: (v) => setState(() => _positionId = v),
            ),
            gap,
            TextField(controller: _state, decoration: const InputDecoration(labelText: 'State')),
            gap,
            TextField(controller: _district, decoration: const InputDecoration(labelText: 'District')),
            gap,
            TextField(controller: _city, decoration: const InputDecoration(labelText: 'City')),
            const SizedBox(height: 20),
            Row(
              children: [
                const Icon(Icons.workspace_premium_outlined, size: 18),
                const SizedBox(width: 6),
                Text('Advanced (premium)', style: Theme.of(context).textTheme.titleSmall),
              ],
            ),
            gap,
            Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _ageMin,
                    keyboardType: TextInputType.number,
                    decoration: const InputDecoration(labelText: 'Min age'),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: TextField(
                    controller: _ageMax,
                    keyboardType: TextInputType.number,
                    decoration: const InputDecoration(labelText: 'Max age'),
                  ),
                ),
              ],
            ),
            gap,
            EnumDropdown(
              label: 'Achievement level',
              values: AchievementLevels.all,
              value: _level,
              allowAny: true,
              onChanged: (v) => setState(() => _level = v),
            ),
            const SizedBox(height: 20),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton(
                    onPressed: () => Navigator.of(context).pop(const PeopleFilters()),
                    child: const Text('Clear'),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(child: FilledButton(onPressed: _apply, child: const Text('Apply'))),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

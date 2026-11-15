import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/format/dates.dart';
import '../../../core/models/sport.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/section.dart';
import '../application/profile_controller.dart';
import '../application/profile_providers.dart';
import '../data/profile_models.dart';
import 'stat_entry_sheet.dart';
import 'stat_format.dart';

/// Stats of [userId], labelled from the sports config templates. The owner can add,
/// edit and delete entries. Career totals appear only when the server sends them.
class StatsScreen extends ConsumerWidget {
  const StatsScreen({super.key, required this.userId});

  final int userId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final isOwn = ref.watch(currentUserIdProvider) == userId;
    final stats = ref.watch(statsProvider(userId));
    return Scaffold(
      appBar: AppBar(title: const Text('Stats')),
      floatingActionButton: isOwn
          ? FloatingActionButton.extended(
              onPressed: () => showStatEntrySheet(context),
              icon: const Icon(Icons.add),
              label: const Text('Add stats'),
            )
          : null,
      body: AsyncValueView<StatsResult>(
        value: stats,
        onRetry: () => ref.invalidate(statsProvider(userId)),
        data: (r) => RefreshIndicator(
          onRefresh: () => ref.refresh(statsProvider(userId).future),
          child: r.entries.isEmpty
              ? ListView(
                  physics: const AlwaysScrollableScrollPhysics(),
                  children: const [SizedBox(height: 240, child: EmptyView(text: 'No stats yet.'))],
                )
              : _StatsList(result: r, isOwn: isOwn),
        ),
      ),
    );
  }
}

class _StatsList extends ConsumerWidget {
  const _StatsList({required this.result, required this.isOwn});

  final StatsResult result;
  final bool isOwn;

  Future<void> _delete(BuildContext context, WidgetRef ref, StatEntry e) async {
    final ok = await confirm(context, title: 'Delete these stats?', body: 'Season ${e.season}', ok: 'Delete');
    if (!ok) return;
    try {
      await ref.read(profileControllerProvider).deleteStat(e.id);
    } catch (err) {
      if (context.mounted) showErrorSnack(context, err);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final sports = ref.watch(sportsProvider);
    final totals = result.totals;
    // Field definitions for the totals, taken from every template used by the entries.
    final fieldsByKey = <String, StatField>{};
    for (final e in result.entries) {
      for (final f in findTemplate(sports, e.sportId, e.templateId)?.template.fields ?? const <StatField>[]) {
        fieldsByKey.putIfAbsent(f.key, () => f);
      }
    }
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.only(bottom: 96),
      children: [
        if (totals != null && totals.isNotEmpty) ...[
          const SectionHeader('Career totals'),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final t in totals.entries)
                  Chip(label: Text('${fieldsByKey[t.key]?.label ?? humanize(t.key)}: ${formatStatValue(fieldsByKey[t.key], t.value)}')),
              ],
            ),
          ),
        ],
        const SectionHeader('Seasons'),
        for (final e in result.entries) _EntryCard(
          entry: e,
          found: findTemplate(sports, e.sportId, e.templateId),
          sportName: ref.watch(bootstrapProvider).valueOrNull?.sportById(e.sportId)?.name,
          onEdit: isOwn ? () => showStatEntrySheet(context, entry: e) : null,
          onDelete: isOwn ? () => _delete(context, ref, e) : null,
        ),
      ],
    );
  }
}

class _EntryCard extends StatelessWidget {
  const _EntryCard({required this.entry, required this.found, required this.sportName, this.onEdit, this.onDelete});

  final StatEntry entry;
  final SportTemplate? found;
  final String? sportName;
  final VoidCallback? onEdit;
  final VoidCallback? onDelete;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final fields = found?.template.fields ?? const <StatField>[];
    final known = {for (final f in fields) f.key};
    final rows = <(String, String)>[
      for (final f in fields)
        if (entry.stats.containsKey(f.key)) (f.label, formatStatValue(f, entry.stats[f.key])),
      // Keys the current template no longer lists are still shown, raw.
      for (final kv in entry.stats.entries)
        if (!known.contains(kv.key)) (humanize(kv.key), '${kv.value}'),
    ];
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 8, 8, 12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    [entry.season, sportName ?? found?.sport.name, found?.template.name].whereType<String>().join(' · '),
                    style: theme.textTheme.titleSmall,
                  ),
                ),
                if (onEdit != null) IconButton(tooltip: 'Edit', icon: const Icon(Icons.edit_outlined), onPressed: onEdit),
                if (onDelete != null)
                  IconButton(tooltip: 'Delete', icon: const Icon(Icons.delete_outline), onPressed: onDelete),
              ],
            ),
            for (final (label, value) in rows)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 2),
                child: Row(
                  children: [
                    Expanded(child: Text(label, style: theme.textTheme.bodyMedium)),
                    Text(value, style: theme.textTheme.bodyLarge),
                  ],
                ),
              ),
            if (entry.updatedAt != null)
              Padding(
                padding: const EdgeInsets.only(top: 6),
                child: Text('Updated ${formatDate(entry.updatedAt)}', style: theme.textTheme.bodySmall),
              ),
          ],
        ),
      ),
    );
  }
}

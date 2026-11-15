import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/format/dates.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../application/profile_controller.dart';
import '../application/profile_providers.dart';
import '../data/profile_models.dart';
import 'achievement_sheet.dart';

/// Achievements of [userId]. The owner can add (up to `profile.max_achievements`),
/// edit and delete, and attach certificates.
class AchievementsScreen extends ConsumerWidget {
  const AchievementsScreen({super.key, required this.userId});

  final int userId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final isOwn = ref.watch(currentUserIdProvider) == userId;
    final list = ref.watch(achievementsProvider(userId));
    final max = ref.watch(appConfigProvider).maxAchievements;
    final count = list.valueOrNull?.length ?? 0;
    final atLimit = max != null && count >= max;
    return Scaffold(
      appBar: AppBar(title: const Text('Achievements')),
      floatingActionButton: isOwn && list.hasValue
          ? FloatingActionButton.extended(
              onPressed: atLimit
                  ? () => showInfoSnack(context, 'You can add up to $max achievements.')
                  : () => showAchievementSheet(context),
              icon: const Icon(Icons.add),
              label: const Text('Add'),
            )
          : null,
      body: AsyncValueView<List<Achievement>>(
        value: list,
        onRetry: () => ref.invalidate(achievementsProvider(userId)),
        data: (items) => RefreshIndicator(
          onRefresh: () => ref.refresh(achievementsProvider(userId).future),
          child: items.isEmpty
              ? ListView(
                  physics: const AlwaysScrollableScrollPhysics(),
                  children: const [SizedBox(height: 240, child: EmptyView(text: 'No achievements yet.'))],
                )
              : ListView(
                  physics: const AlwaysScrollableScrollPhysics(),
                  padding: const EdgeInsets.only(bottom: 96),
                  children: [
                    if (isOwn && max != null)
                      Padding(
                        padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
                        child: Text('$count of $max', style: Theme.of(context).textTheme.bodySmall),
                      ),
                    for (final a in items) AchievementTile(achievement: a, editable: isOwn),
                  ],
                ),
        ),
      ),
    );
  }
}

/// One achievement row; with [editable] it offers edit and delete.
class AchievementTile extends ConsumerWidget {
  const AchievementTile({super.key, required this.achievement, this.editable = false});

  final Achievement achievement;
  final bool editable;

  Future<void> _openCertificate(BuildContext context) async {
    final uri = Uri.tryParse(achievement.certificateUrl ?? '');
    if (uri == null) return;
    final ok = await launchUrl(uri, mode: LaunchMode.externalApplication);
    if (!ok && context.mounted) showInfoSnack(context, 'Could not open the certificate.');
  }

  Future<void> _delete(BuildContext context, WidgetRef ref) async {
    final ok = await confirm(context, title: 'Delete "${achievement.title}"?', ok: 'Delete');
    if (!ok) return;
    try {
      await ref.read(profileControllerProvider).deleteAchievement(achievement.id);
    } catch (e) {
      if (context.mounted) showErrorSnack(context, e);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final a = achievement;
    final sport = ref.watch(bootstrapProvider).valueOrNull?.sportById(a.sportId)?.name;
    final subtitle = [humanize(a.level), if (sport != null) sport, if (a.achievedOn != null) formatDate(a.achievedOn)].join(' · ');
    return ListTile(
      leading: const Icon(Icons.emoji_events_outlined),
      title: Text(a.title),
      subtitle: Text([subtitle, if (a.description != null && a.description!.isNotEmpty) a.description!].join('\n')),
      isThreeLine: a.description != null && a.description!.isNotEmpty,
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (a.certificateUrl != null)
            IconButton(
              tooltip: 'Certificate',
              icon: const Icon(Icons.description_outlined),
              onPressed: () => _openCertificate(context),
            ),
          if (editable)
            PopupMenuButton<String>(
              onSelected: (v) => v == 'edit' ? showAchievementSheet(context, existing: a) : _delete(context, ref),
              itemBuilder: (_) => const [
                PopupMenuItem(value: 'edit', child: Text('Edit')),
                PopupMenuItem(value: 'delete', child: Text('Delete')),
              ],
            ),
        ],
      ),
    );
  }
}

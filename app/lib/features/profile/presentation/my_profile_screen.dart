import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/page_hero.dart';
import '../../../core/widgets/section.dart';
import '../application/profile_providers.dart';
import '../data/profile_models.dart';
import 'achievement_sheet.dart';
import 'achievements_screen.dart';
import 'profile_header.dart';
import 'stat_entry_sheet.dart';

/// The Me tab: own profile, quick stat entry from the sports config templates,
/// achievements, profile viewers and links to the user's other areas.
class MyProfileScreen extends ConsumerWidget {
  const MyProfileScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final profile = ref.watch(myProfileProvider);
    return Scaffold(
      backgroundColor: Colors.transparent,
      body: Column(
        children: [
          PageHero(
            eyebrow: 'Your game. Your story.',
            title: 'My profile',
            trailing: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                HeroIconButton(
                  tooltip: 'Edit profile',
                  icon: const Icon(Icons.edit_outlined),
                  onPressed: () => context.push(Routes.meEdit),
                ),
                const SizedBox(width: 8),
                HeroIconButton(
                  tooltip: 'Settings',
                  icon: const Icon(Icons.settings_outlined),
                  onPressed: () => context.push(Routes.settings),
                ),
              ],
            ),
          ),
          Expanded(child: AsyncValueView<Profile?>(
        value: profile,
        onRetry: () => ref.invalidate(myProfileProvider),
        data: (p) => p == null
            ? const EmptyView(text: 'Sign in to see your profile.')
            : RefreshIndicator(
                onRefresh: () async {
                  ref.invalidate(profileProvider(p.user.id));
                  ref.invalidate(achievementsProvider(p.user.id));
                  await ref.read(myProfileProvider.future);
                },
                child: _MyProfileBody(profile: p),
              ),
          )),
        ],
      ),
    );
  }
}

class _MyProfileBody extends ConsumerWidget {
  const _MyProfileBody({required this.profile});

  final Profile profile;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final id = profile.user.id;
    final me = ref.watch(meProvider);
    final templates = templatesForProfile(ref.watch(sportsProvider), profile);
    final achievements = ref.watch(achievementsProvider(id));
    final maxAchievements = ref.watch(appConfigProvider).maxAchievements;
    final achievementCount = achievements.valueOrNull?.length ?? 0;
    final canAddAchievement = achievements.hasValue && (maxAchievements == null || achievementCount < maxAchievements);
    final isOrganiser = me?.isOrganiser ?? false;

    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.only(bottom: 24),
      children: [
        ProfileHeader(
          profile: profile,
          actions: [
            OutlinedButton.icon(
              onPressed: () => context.push(Routes.meEdit),
              icon: const Icon(Icons.edit_outlined),
              label: const Text('Edit profile'),
            ),
            OutlinedButton.icon(
              onPressed: () => context.push(Routes.profile(id)),
              icon: const Icon(Icons.visibility_outlined),
              label: const Text('View as others'),
            ),
          ],
        ),
        if (profile.bio != null && profile.bio!.isNotEmpty)
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 0),
            child: Text(profile.bio!, style: const TextStyle(height: 1.6)),
          ),

        SectionHeader(
          'Stats',
          trailing: TextButton(onPressed: () => context.push(Routes.stats(id)), child: const Text('See all')),
        ),
        if (templates.isEmpty)
          ListTile(
            leading: const Icon(Icons.info_outline),
            title: const Text('Add a sport to your profile to record stats.'),
            onTap: () => context.push(Routes.meEdit),
          )
        else
          for (final t in templates)
            ListTile(
              leading: const Icon(Icons.add_chart_outlined),
              title: Text(t.template.name),
              subtitle: Text([t.sport.name, t.template.fields.map((f) => f.label).take(4).join(', ')].join(' · ')),
              trailing: const Icon(Icons.add),
              onTap: () => showStatEntrySheet(context, sportId: t.sport.id, templateId: t.template.id),
            ),

        SectionHeader(
          'Achievements',
          trailing: TextButton(onPressed: () => context.push(Routes.achievements(id)), child: const Text('See all')),
        ),
        ...achievements.when<List<Widget>>(
          skipLoadingOnRefresh: true,
          loading: () => const [Padding(padding: EdgeInsets.all(16), child: Center(child: CircularProgressIndicator()))],
          error: (_, __) => [
            ListTile(
              leading: const Icon(Icons.error_outline),
              title: const Text('Could not load achievements.'),
              trailing: TextButton(onPressed: () => ref.invalidate(achievementsProvider(id)), child: const Text('Retry')),
            ),
          ],
          data: (items) => [for (final a in items.take(3)) AchievementTile(achievement: a, editable: true)],
        ),
        if (canAddAchievement)
          ListTile(
            leading: const Icon(Icons.add),
            title: const Text('Add achievement'),
            subtitle: const Text('Attach a certificate to show it is genuine'),
            onTap: () => showAchievementSheet(context),
          )
        else if (achievements.hasValue && maxAchievements != null)
          ListTile(
            leading: const Icon(Icons.info_outline),
            title: Text('You have reached the limit of $maxAchievements achievements.'),
          ),

        const SectionHeader('Activity'),
        _link(context, Icons.visibility_outlined, 'Who viewed my profile', Routes.meViewers),
        _link(context, Icons.dynamic_feed_outlined, 'My posts', Routes.userPosts(id)),
        _link(context, Icons.confirmation_number_outlined, 'My registrations', Routes.myRegistrations),
        _link(context, Icons.groups_outlined, 'Communities', Routes.communities),
        _link(context, Icons.sports_outlined, 'Consultations', Routes.consultations),
        _link(context, Icons.workspace_premium_outlined, 'Premium', Routes.premium),
        if (isOrganiser) ...[
          _link(context, Icons.event_note_outlined, 'My events', Routes.myEvents),
          _link(context, Icons.verified_user_outlined, 'Organiser verification', Routes.organiserVerification),
        ],
        _link(context, Icons.settings_outlined, 'Settings', Routes.settings),
      ],
    );
  }

  Widget _link(BuildContext context, IconData icon, String title, String route) => ListTile(
        leading: Icon(icon),
        title: Text(title),
        trailing: const Icon(Icons.chevron_right),
        onTap: () => context.push(route),
      );
}

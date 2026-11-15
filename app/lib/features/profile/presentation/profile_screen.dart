import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/section.dart';
import '../../chat/data/chat_api.dart';
import '../../follow/presentation/block_actions.dart';
import '../../follow/presentation/follow_button.dart';
import '../../reports/presentation/report_sheet.dart';
import '../application/profile_providers.dart';
import '../data/profile_models.dart';
import 'profile_header.dart';

/// Someone's public profile. Blocked either way → 404 (shown as an error).
class ProfileScreen extends ConsumerWidget {
  const ProfileScreen({super.key, required this.userId});

  final int userId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final profile = ref.watch(profileProvider(userId));
    final p = profile.valueOrNull;
    return Scaffold(
      appBar: AppBar(
        title: Text(p?.user.displayName ?? 'Profile'),
        actions: [
          if (p != null && !p.isOwn) _OverflowMenu(profile: p),
        ],
      ),
      body: AsyncValueView<Profile>(
        value: profile,
        onRetry: () => ref.invalidate(profileProvider(userId)),
        data: (p) => RefreshIndicator(
          onRefresh: () => ref.refresh(profileProvider(userId).future),
          child: ProfileBody(profile: p),
        ),
      ),
    );
  }
}

enum _MenuAction { report, block, unblock }

class _OverflowMenu extends ConsumerWidget {
  const _OverflowMenu({required this.profile});

  final Profile profile;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final u = profile.user;
    return PopupMenuButton<_MenuAction>(
      onSelected: (a) async {
        switch (a) {
          case _MenuAction.report:
            await showReportSheet(context, targetType: ReportTarget.user, targetId: u.id);
          case _MenuAction.block:
            final done = await confirmBlock(context, ref, userId: u.id, name: u.displayName);
            // A blocked profile is no longer visible (404), so leave it.
            if (done && context.mounted) context.pop();
          case _MenuAction.unblock:
            await confirmUnblock(context, ref, userId: u.id, name: u.displayName);
        }
      },
      itemBuilder: (_) => [
        const PopupMenuItem(value: _MenuAction.report, child: Text('Report')),
        if (profile.isBlockedByMe)
          const PopupMenuItem(value: _MenuAction.unblock, child: Text('Unblock'))
        else
          const PopupMenuItem(value: _MenuAction.block, child: Text('Block')),
      ],
    );
  }
}

/// Profile content without the scaffold (also used by the Me tab for its header part).
class ProfileBody extends ConsumerWidget {
  const ProfileBody({super.key, required this.profile});

  final Profile profile;

  Future<void> _message(BuildContext context, WidgetRef ref) async {
    final conversation = await ref.read(chatApiProvider).direct(profile.user.id);
    if (context.mounted) context.push(Routes.conversation(conversation.id));
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final p = profile;
    final id = p.user.id;
    final actions = <Widget>[
      if (p.isOwn)
        OutlinedButton.icon(
          onPressed: () => context.push(Routes.meEdit),
          icon: const Icon(Icons.edit_outlined),
          label: const Text('Edit profile'),
        )
      else ...[
        FollowButton(
          key: ValueKey('follow-$id-${p.isFollowing}'),
          userId: id,
          initialFollowing: p.isFollowing,
        ),
        if (p.canMessage)
          BusyButton(
            label: 'Message',
            icon: Icons.chat_bubble_outline,
            style: BusyButtonStyle.outlined,
            onPressed: () => _message(context, ref),
          ),
        if (p.consultationOffered ?? false)
          OutlinedButton.icon(
            onPressed: () => context.push(Routes.coachConsultation(id)),
            icon: const Icon(Icons.sports_outlined),
            label: const Text('Book consultation'),
          ),
      ],
    ];
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.only(bottom: 24),
      children: [
        ProfileHeader(profile: p, actions: actions),
        if (p.restricted)
          const Padding(
            padding: EdgeInsets.all(24),
            child: Column(
              children: [
                Icon(Icons.lock_outline, size: 36),
                SizedBox(height: 8),
                Text('This profile is private.', textAlign: TextAlign.center),
              ],
            ),
          )
        else ...[
          if (p.bio != null && p.bio!.isNotEmpty)
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
              child: Text(p.bio!),
            ),
          if (p.sports.isNotEmpty) ...[
            const SectionHeader('Sports'),
            for (final s in p.sports)
              ListTile(
                leading: Icon(s.isPrimary ? Icons.star : Icons.sports),
                title: Text(s.sport.name),
                subtitle: s.position == null ? null : Text(s.position!.name),
              ),
          ],
          ..._details(p),
          const SectionHeader('More'),
          ListTile(
            leading: const Icon(Icons.bar_chart_outlined),
            title: const Text('Stats'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => context.push(Routes.stats(id)),
          ),
          ListTile(
            leading: const Icon(Icons.emoji_events_outlined),
            title: const Text('Achievements'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => context.push(Routes.achievements(id)),
          ),
          ListTile(
            leading: const Icon(Icons.dynamic_feed_outlined),
            title: const Text('Posts'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => context.push(Routes.userPosts(id)),
          ),
        ],
      ],
    );
  }

  List<Widget> _details(Profile p) {
    final rows = <Widget>[
      if (p.academyClub != null && p.academyClub!.isNotEmpty)
        InfoRow(label: 'Academy / club', value: p.academyClub!, icon: Icons.school_outlined),
      if (p.district != null && p.district!.isNotEmpty)
        InfoRow(label: 'District', value: p.district!, icon: Icons.place_outlined),
      if (p.user.state != null && p.user.state!.isNotEmpty)
        InfoRow(label: 'State', value: p.user.state!, icon: Icons.map_outlined),
      if (p.coachingSinceYear != null)
        InfoRow(label: 'Coaching since', value: '${p.coachingSinceYear}', icon: Icons.sports_outlined),
      if (p.organiser != null)
        InfoRow(
          label: 'Organiser',
          value: [
            if (p.organiser!.orgName != null) p.organiser!.orgName!,
            if (p.organiser!.orgType != null) humanize(p.organiser!.orgType!),
            humanize(p.organiser!.status),
          ].join(' · '),
          icon: Icons.business_outlined,
        ),
    ];
    if (rows.isEmpty) return const [];
    return [const SectionHeader('About'), ...rows];
  }
}

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/section.dart';
import '../../../core/widgets/user_avatar.dart';
import '../../../core/widgets/user_tile.dart';
import '../../reports/presentation/report_sheet.dart';
import '../application/community_providers.dart';
import '../data/community_models.dart';

/// A community: info, and the action that fits `myStatus` / `myRole`.
class CommunityDetailScreen extends ConsumerWidget {
  const CommunityDetailScreen({super.key, required this.communityId});

  final int communityId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(communityProvider(communityId));
    final c = value.valueOrNull;
    return Scaffold(
      appBar: AppBar(
        title: Text(c?.title ?? 'Community'),
        actions: [
          if (c != null && c.canModerate)
            IconButton(
              tooltip: 'Edit',
              icon: const Icon(Icons.edit_outlined),
              onPressed: () => context.push(Routes.communityEdit(c.id)),
            ),
          if (c != null && c.canModerate)
            IconButton(
              tooltip: 'Manage',
              icon: const Icon(Icons.manage_accounts_outlined),
              onPressed: () => context.push(Routes.communityManage(c.id)),
            ),
          if (c != null)
            IconButton(
              tooltip: 'Report',
              icon: const Icon(Icons.flag_outlined),
              onPressed: () => showReportSheet(context, targetType: ReportTarget.community, targetId: c.id),
            ),
        ],
      ),
      body: AsyncValueView<Community>(
        value: value,
        onRetry: () => ref.invalidate(communityProvider(communityId)),
        data: (c) => RefreshIndicator(
          onRefresh: () => ref.refresh(communityProvider(communityId).future),
          child: ListView(
            padding: const EdgeInsets.only(bottom: 24),
            children: [
              const SizedBox(height: 16),
              Center(child: UserAvatar(name: c.title, url: c.avatarUrl, radius: 44)),
              const SizedBox(height: 12),
              Center(child: Text(c.title, style: Theme.of(context).textTheme.titleLarge, textAlign: TextAlign.center)),
              const SizedBox(height: 4),
              Center(
                child: Text(
                  [
                    humanize(c.visibility),
                    '${humanize(c.joinPolicy)} to join',
                    '${c.memberCount} members',
                  ].join(' · '),
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ),
              Padding(padding: const EdgeInsets.all(16), child: _Actions(community: c)),
              if (c.description != null && c.description!.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 16),
                  child: Text(c.description!),
                ),
              const SectionHeader('Details'),
              InfoRow(label: 'Sport', value: c.sport?.name ?? '', icon: Icons.sports_outlined),
              InfoRow(label: 'Location', value: c.place, icon: Icons.place_outlined),
              InfoRow(label: 'Created', value: formatDate(c.createdAt), icon: Icons.calendar_today_outlined),
              if (c.owner != null) ...[
                const SectionHeader('Owner'),
                UserTile(user: c.owner!, onTap: () => context.push(Routes.profile(c.owner!.id))),
              ],
              if (c.isMember)
                ListTile(
                  leading: const Icon(Icons.people_outline),
                  title: const Text('Members'),
                  trailing: const Icon(Icons.chevron_right),
                  onTap: () => context.push(Routes.conversationMembers(c.id)),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

class _Actions extends ConsumerWidget {
  const _Actions({required this.community});

  final Community community;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = community;
    final ctl = ref.read(communityControllerProvider);
    switch (c.myStatus) {
      case MembershipStatus.active:
        return Row(
          children: [
            Expanded(
              child: FilledButton.icon(
                onPressed: () => context.push(Routes.conversation(c.id)),
                icon: const Icon(Icons.forum_outlined),
                label: const Text('Open chat'),
              ),
            ),
            if (!c.isOwner) ...[
              const SizedBox(width: 8),
              Expanded(
                child: BusyButton(
                  label: 'Leave',
                  style: BusyButtonStyle.outlined,
                  onPressed: () async {
                    final ok = await confirm(context, title: 'Leave ${c.title}?', ok: 'Leave');
                    if (ok) await ctl.leave(c.id);
                  },
                ),
              ),
            ],
          ],
        );
      case MembershipStatus.invited:
        return BusyButton(
          label: 'Accept invite',
          icon: Icons.mail_outline,
          onPressed: () async {
            await ctl.acceptInvite(c.id);
            if (context.mounted) showInfoSnack(context, 'You joined ${c.title}.');
          },
        );
      case MembershipStatus.pending:
        return const ListTile(
          leading: Icon(Icons.hourglass_top),
          title: Text('Join request sent'),
          subtitle: Text('An admin will review it.'),
        );
      case MembershipStatus.banned:
        return const ListTile(
          leading: Icon(Icons.block),
          title: Text('You can no longer join this community.'),
        );
      default:
        if (c.joinPolicy == JoinPolicy.invite) {
          return const ListTile(
            leading: Icon(Icons.lock_outline),
            title: Text('Invite only'),
            subtitle: Text('Ask a member to invite you.'),
          );
        }
        return BusyButton(
          label: c.joinPolicy == JoinPolicy.approval ? 'Request to join' : 'Join',
          icon: Icons.group_add_outlined,
          onPressed: () async {
            final status = await ctl.join(c.id);
            if (!context.mounted) return;
            showInfoSnack(
              context,
              status == MembershipStatus.active ? 'You joined ${c.title}.' : 'Request sent.',
            );
          },
        );
    }
  }
}

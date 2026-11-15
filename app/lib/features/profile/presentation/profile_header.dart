import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/user_avatar.dart';
import '../data/profile_models.dart';

/// Avatar, name, badges and follower counts. Shared by the profile and Me screens.
class ProfileHeader extends StatelessWidget {
  const ProfileHeader({super.key, required this.profile, this.actions = const []});

  final Profile profile;
  final List<Widget> actions;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final u = profile.user;
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              UserAvatar(name: u.displayName, url: u.photoUrl, radius: 36),
              const SizedBox(width: 16),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Flexible(
                          child: Text(u.displayName, style: theme.textTheme.titleLarge, overflow: TextOverflow.ellipsis),
                        ),
                        if (u.isVerified) ...[
                          const SizedBox(width: 4),
                          Icon(Icons.verified, size: 18, color: theme.colorScheme.primary),
                        ],
                      ],
                    ),
                    if (u.primaryRole != null) Text(humanize(u.primaryRole!), style: theme.textTheme.bodyMedium),
                    if (u.subtitle.isNotEmpty) Text(u.subtitle, style: theme.textTheme.bodySmall),
                  ],
                ),
              ),
            ],
          ),
          if (profile.badges.isNotEmpty) ...[
            const SizedBox(height: 8),
            Wrap(
              spacing: 6,
              children: [
                for (final b in profile.badges)
                  Chip(
                    avatar: const Icon(Icons.workspace_premium, size: 16),
                    label: Text(humanize(b)),
                    visualDensity: VisualDensity.compact,
                  ),
              ],
            ),
          ],
          const SizedBox(height: 8),
          Row(
            children: [
              _Count(
                label: 'Followers',
                value: profile.followersCount,
                onTap: () => context.push(Routes.followers(u.id)),
              ),
              const SizedBox(width: 24),
              _Count(
                label: 'Following',
                value: profile.followingCount,
                onTap: () => context.push(Routes.following(u.id)),
              ),
              if (profile.isFollowedBy && !profile.isOwn) ...[
                const SizedBox(width: 16),
                Chip(label: const Text('Follows you'), visualDensity: VisualDensity.compact),
              ],
            ],
          ),
          if (actions.isNotEmpty) ...[
            const SizedBox(height: 12),
            Wrap(spacing: 8, runSpacing: 8, children: actions),
          ],
        ],
      ),
    );
  }
}

class _Count extends StatelessWidget {
  const _Count({required this.label, required this.value, required this.onTap});

  final String label;
  final int value;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(8),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 4, horizontal: 2),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('$value', style: theme.textTheme.titleMedium),
            Text(label, style: theme.textTheme.bodySmall),
          ],
        ),
      ),
    );
  }
}

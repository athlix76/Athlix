import 'package:flutter/material.dart';

import '../../../core/format/dates.dart';
import '../../../core/widgets/user_avatar.dart';
import '../data/community_models.dart';

/// One community in a list.
class CommunityTile extends StatelessWidget {
  const CommunityTile({super.key, required this.community, this.onTap});

  final Community community;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final c = community;
    final parts = [
      if (c.sport != null) c.sport!.name,
      if (c.place.isNotEmpty) c.place,
      '${c.memberCount} members',
    ];
    return ListTile(
      onTap: onTap,
      leading: UserAvatar(name: c.title, url: c.avatarUrl),
      title: Row(
        children: [
          Flexible(child: Text(c.title, overflow: TextOverflow.ellipsis)),
          if (c.visibility == CommunityVisibility.private) ...[
            const SizedBox(width: 4),
            const Icon(Icons.lock_outline, size: 16),
          ],
        ],
      ),
      subtitle: Text(parts.join(' · '), maxLines: 1, overflow: TextOverflow.ellipsis),
      trailing: c.myStatus == MembershipStatus.none ? null : Chip(label: Text(statusLabel(c))),
    );
  }

  static String statusLabel(Community c) => switch (c.myStatus) {
        MembershipStatus.active => c.myRole == null || c.myRole == 'member' ? 'Member' : humanize(c.myRole!),
        MembershipStatus.pending => 'Requested',
        MembershipStatus.invited => 'Invited',
        MembershipStatus.banned => 'Banned',
        _ => '',
      };
}

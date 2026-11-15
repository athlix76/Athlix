import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../models/user_summary.dart';
import '../router/routes.dart';
import 'user_avatar.dart';

/// The standard way to show another user in a list.
class UserTile extends StatelessWidget {
  const UserTile({super.key, required this.user, this.trailing, this.onTap, this.subtitle});

  final UserSummary user;
  final Widget? trailing;
  final VoidCallback? onTap;
  final String? subtitle;

  @override
  Widget build(BuildContext context) {
    final sub = subtitle ?? user.subtitle;
    return ListTile(
      leading: UserAvatar(name: user.displayName, url: user.photoUrl),
      title: Row(
        children: [
          Flexible(child: Text(user.displayName, overflow: TextOverflow.ellipsis)),
          if (user.isVerified) ...[
            const SizedBox(width: 4),
            Icon(Icons.verified, size: 16, color: Theme.of(context).colorScheme.primary),
          ],
        ],
      ),
      subtitle: sub.isEmpty ? null : Text(sub),
      trailing: trailing,
      onTap: onTap ?? () => context.push(Routes.profile(user.id)),
    );
  }
}

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/models/user_summary.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/user_tile.dart';
import '../data/follow_api.dart';

/// Followers (or, with [following] true, the people [userId] follows).
class FollowListScreen extends ConsumerWidget {
  const FollowListScreen({super.key, required this.userId, required this.following});

  final int userId;
  final bool following;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final api = ref.watch(followApiProvider);
    return Scaffold(
      appBar: AppBar(title: Text(following ? 'Following' : 'Followers')),
      body: CursorListView<UserSummary>(
        key: ValueKey('$userId-$following'),
        fetch: (cursor) => following ? api.following(userId, cursor: cursor) : api.followers(userId, cursor: cursor),
        emptyText: following ? 'Not following anyone yet.' : 'No followers yet.',
        separated: true,
        itemBuilder: (context, u, _) => UserTile(user: u),
      ),
    );
  }
}

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/models/user_summary.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/user_tile.dart';
import '../data/follow_api.dart';
import 'block_actions.dart';

/// Settings → Blocked users. Blocked profiles return 404, so rows do not open a profile.
class BlockedUsersScreen extends ConsumerStatefulWidget {
  const BlockedUsersScreen({super.key});

  @override
  ConsumerState<BlockedUsersScreen> createState() => _BlockedUsersScreenState();
}

class _BlockedUsersScreenState extends ConsumerState<BlockedUsersScreen> {
  final _list = CursorListController<UserSummary>();

  Future<void> _unblock(UserSummary u) async {
    final done = await confirmUnblock(context, ref, userId: u.id, name: u.displayName);
    if (done) _list.removeWhere((x) => x.id == u.id);
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(followApiProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Blocked users')),
      body: CursorListView<UserSummary>(
        controller: _list,
        fetch: (cursor) => api.blocks(cursor: cursor),
        emptyText: 'You have not blocked anyone.',
        separated: true,
        itemBuilder: (context, u, _) => UserTile(
          user: u,
          onTap: () {},
          trailing: TextButton(onPressed: () => _unblock(u), child: const Text('Unblock')),
        ),
      ),
    );
  }
}

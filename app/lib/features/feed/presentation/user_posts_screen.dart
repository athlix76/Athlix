import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/widgets/cursor_list_view.dart';
import '../data/feed_api.dart';
import '../data/post_models.dart';
import 'post_card.dart';

/// `/profile/:id/posts`: one user's posts (the server applies visibility and blocks).
class UserPostsScreen extends ConsumerStatefulWidget {
  const UserPostsScreen({super.key, required this.userId});

  final int userId;

  @override
  ConsumerState<UserPostsScreen> createState() => _UserPostsScreenState();
}

class _UserPostsScreenState extends ConsumerState<UserPostsScreen> {
  final _list = CursorListController<Post>();

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(feedApiProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Posts')),
      body: CursorListView<Post>(
        key: ValueKey(widget.userId),
        controller: _list,
        fetch: (cursor) => api.userPosts(widget.userId, cursor: cursor),
        emptyText: 'No posts yet.',
        itemBuilder: (context, post, _) => PostCard(
          post: post,
          onChanged: (p) => _list.replaceWhere((x) => x.id == p.id, (_) => p),
          onDeleted: () => _list.removeWhere((x) => x.id == post.id),
        ),
      ),
    );
  }
}

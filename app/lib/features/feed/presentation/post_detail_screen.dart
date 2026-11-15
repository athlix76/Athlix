import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/user_avatar.dart';
import '../../reports/presentation/report_sheet.dart';
import '../application/feed_controller.dart';
import '../data/feed_api.dart';
import '../data/post_models.dart';
import 'post_card.dart';

/// `/posts/:id`: the post, its comments (oldest first) and a reply box.
/// Comments can be deleted by their author or by the post's author.
class PostDetailScreen extends ConsumerStatefulWidget {
  const PostDetailScreen({super.key, required this.postId});

  final int postId;

  @override
  ConsumerState<PostDetailScreen> createState() => _PostDetailScreenState();
}

class _PostDetailScreenState extends ConsumerState<PostDetailScreen> {
  final _comments = CursorListController<Comment>();
  final _input = TextEditingController();

  /// Local copy after likes / comment counts change.
  Post? _post;
  bool _sending = false;

  @override
  void dispose() {
    _input.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    final text = _input.text.trim();
    if (text.isEmpty || _sending) return;
    setState(() => _sending = true);
    try {
      await ref.read(feedControllerProvider).addComment(widget.postId, text);
      _input.clear();
      final p = _post;
      if (p != null) setState(() => _post = p.copyWith(commentCount: p.commentCount + 1));
      await _comments.refresh();
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  Future<void> _deleteComment(Comment c) async {
    final ok = await confirm(context, title: 'Delete this comment?', ok: 'Delete');
    if (!ok || !mounted) return;
    try {
      await ref.read(feedControllerProvider).deleteComment(widget.postId, c.id);
      _comments.removeWhere((x) => x.id == c.id);
      final p = _post;
      if (p != null && p.commentCount > 0) {
        setState(() => _post = p.copyWith(commentCount: p.commentCount - 1));
      }
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  @override
  Widget build(BuildContext context) {
    final value = ref.watch(postProvider(widget.postId));
    final api = ref.watch(feedApiProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Post')),
      body: AsyncValueView<Post>(
        value: value,
        onRetry: () => ref.invalidate(postProvider(widget.postId)),
        data: (loaded) {
          final post = _post?.id == loaded.id ? _post! : loaded;
          return Column(
            children: [
              Expanded(
                child: CursorListView<Comment>(
                  controller: _comments,
                  fetch: (cursor) => api.comments(widget.postId, cursor: cursor),
                  emptyText: 'No comments yet.',
                  header: PostCard(
                    post: post,
                    openOnTap: false,
                    onChanged: (p) => setState(() => _post = p),
                    onDeleted: () {
                      if (context.mounted) context.pop();
                    },
                  ),
                  itemBuilder: (context, c, _) => _CommentTile(
                    comment: c,
                    canDelete: c.isOwn || post.isOwn,
                    onDelete: () => _deleteComment(c),
                  ),
                ),
              ),
              SafeArea(
                top: false,
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(12, 4, 4, 8),
                  child: Row(
                    children: [
                      Expanded(
                        child: TextField(
                          controller: _input,
                          minLines: 1,
                          maxLines: 4,
                          textInputAction: TextInputAction.send,
                          onSubmitted: (_) => _send(),
                          decoration: const InputDecoration(hintText: 'Write a comment', isDense: true),
                        ),
                      ),
                      IconButton(
                        tooltip: 'Send',
                        onPressed: _sending ? null : _send,
                        icon: _sending
                            ? const SizedBox(
                                width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
                            : const Icon(Icons.send),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}

class _CommentTile extends StatelessWidget {
  const _CommentTile({required this.comment, required this.canDelete, required this.onDelete});

  final Comment comment;
  final bool canDelete;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    final c = comment;
    return ListTile(
      leading: GestureDetector(
        onTap: () => context.push(Routes.profile(c.author.id)),
        child: UserAvatar(name: c.author.displayName, url: c.author.photoUrl, radius: 16),
      ),
      title: Text('${c.author.displayName} · ${timeAgo(c.createdAt)}',
          style: Theme.of(context).textTheme.labelMedium),
      subtitle: Text(c.body, style: Theme.of(context).textTheme.bodyMedium),
      trailing: PopupMenuButton<String>(
        onSelected: (v) {
          if (v == 'delete') onDelete();
          if (v == 'report') {
            showReportSheet(context, targetType: ReportTarget.comment, targetId: c.id);
          }
        },
        itemBuilder: (_) => [
          if (canDelete) const PopupMenuItem(value: 'delete', child: Text('Delete')),
          if (!c.isOwn) const PopupMenuItem(value: 'report', child: Text('Report')),
        ],
      ),
    );
  }
}

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/user_avatar.dart';
import '../../reports/presentation/report_sheet.dart';
import '../application/feed_controller.dart';
import '../data/post_models.dart';

/// One post in a list or on the detail screen. Reports go through the shared
/// report sheet; delete is offered only for the author's own posts.
class PostCard extends ConsumerStatefulWidget {
  const PostCard({
    super.key,
    required this.post,
    this.onChanged,
    this.onDeleted,
    this.openOnTap = true,
  });

  final Post post;

  /// Called with the updated post after a like / unlike.
  final ValueChanged<Post>? onChanged;
  final VoidCallback? onDeleted;

  /// False on the detail screen (it is already open).
  final bool openOnTap;

  @override
  ConsumerState<PostCard> createState() => _PostCardState();
}

class _PostCardState extends ConsumerState<PostCard> {
  bool _liking = false;

  Post get p => widget.post;

  Future<void> _toggleLike() async {
    if (_liking) return;
    final before = p;
    setState(() => _liking = true);
    // Optimistic: flip now, then take the server's count.
    widget.onChanged?.call(before.copyWith(
      likedByMe: !before.likedByMe,
      likeCount: before.likeCount + (before.likedByMe ? -1 : 1),
    ));
    try {
      final updated = await ref.read(feedControllerProvider).toggleLike(before);
      widget.onChanged?.call(updated);
    } catch (e) {
      widget.onChanged?.call(before);
      if (mounted) showErrorSnack(context, e);
    } finally {
      if (mounted) setState(() => _liking = false);
    }
  }

  Future<void> _delete() async {
    final ok = await confirm(context, title: 'Delete this post?', ok: 'Delete');
    if (!ok || !mounted) return;
    try {
      await ref.read(feedControllerProvider).delete(p.id);
      widget.onDeleted?.call();
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final body = p.body;
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: widget.openOnTap ? () => context.push(Routes.post(p.id)) : null,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            ListTile(
              leading: UserAvatar(name: p.author.displayName, url: p.author.photoUrl),
              title: Row(
                children: [
                  Flexible(child: Text(p.author.displayName, overflow: TextOverflow.ellipsis)),
                  if (p.author.isVerified) ...[
                    const SizedBox(width: 4),
                    Icon(Icons.verified, size: 16, color: theme.colorScheme.primary),
                  ],
                ],
              ),
              subtitle: Text([
                timeAgo(p.createdAt),
                if (p.visibility == PostVisibility.followers) 'Followers',
              ].join(' · ')),
              onTap: () => context.push(Routes.profile(p.author.id)),
              trailing: PopupMenuButton<String>(
                onSelected: (v) {
                  if (v == 'delete') _delete();
                  if (v == 'report') {
                    showReportSheet(context, targetType: ReportTarget.post, targetId: p.id);
                  }
                },
                itemBuilder: (_) => [
                  if (p.isOwn) const PopupMenuItem(value: 'delete', child: Text('Delete'))
                  else const PopupMenuItem(value: 'report', child: Text('Report')),
                ],
              ),
            ),
            if (body != null && body.isNotEmpty)
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                child: Text(body),
              ),
            if (p.images.isNotEmpty) _Images(urls: p.images),
            if (p.sport != null || p.eventId != null)
              Padding(
                padding: const EdgeInsets.fromLTRB(12, 8, 12, 0),
                child: Wrap(
                  spacing: 8,
                  children: [
                    if (p.sport != null) Chip(label: Text(p.sport!.name)),
                    if (p.eventId != null)
                      ActionChip(
                        avatar: const Icon(Icons.event, size: 18),
                        label: const Text('View event'),
                        onPressed: () => context.push(Routes.event(p.eventId!)),
                      ),
                  ],
                ),
              ),
            Row(
              children: [
                TextButton.icon(
                  onPressed: _liking ? null : _toggleLike,
                  icon: Icon(
                    p.likedByMe ? Icons.favorite : Icons.favorite_border,
                    color: p.likedByMe ? theme.colorScheme.error : null,
                  ),
                  label: Text('${p.likeCount}'),
                ),
                TextButton.icon(
                  onPressed: widget.openOnTap ? () => context.push(Routes.post(p.id)) : null,
                  icon: const Icon(Icons.mode_comment_outlined),
                  label: Text('${p.commentCount}'),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _Images extends StatelessWidget {
  const _Images({required this.urls});

  final List<String> urls;

  @override
  Widget build(BuildContext context) {
    if (urls.length == 1) return SignedImage(url: urls.first, height: 280);
    return SizedBox(
      height: 280,
      child: PageView.builder(
        itemCount: urls.length,
        itemBuilder: (_, i) => Stack(
          fit: StackFit.expand,
          children: [
            SignedImage(url: urls[i], height: 280),
            Positioned(
              right: 8,
              top: 8,
              child: DecoratedBox(
                decoration: BoxDecoration(
                  color: Colors.black54,
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                  child: Text('${i + 1}/${urls.length}', style: const TextStyle(color: Colors.white)),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

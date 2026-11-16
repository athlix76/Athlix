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
    final a = p.author;
    final sport = a.sport?.name;
    final rest = [
      if ((a.city ?? '').isNotEmpty) a.city!,
      timeAgo(p.createdAt),
      if (p.visibility == PostVisibility.followers) 'Followers',
    ].where((s) => s.isNotEmpty).join(' · ');
    return InkWell(
      onTap: widget.openOnTap ? () => context.push(Routes.post(p.id)) : null,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 20, 12, 16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Author row: ringed avatar, name (+ verified), sport (cyan) · city · time, menu.
            InkWell(
              borderRadius: BorderRadius.circular(12),
              onTap: () => context.push(Routes.profile(a.id)),
              child: Row(
                children: [
                  Container(
                    padding: const EdgeInsets.all(3),
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      border: Border.all(color: theme.colorScheme.primary, width: 2.5),
                    ),
                    child: UserAvatar(name: a.displayName, url: a.photoUrl, radius: 23),
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          children: [
                            Flexible(
                              child: Text(
                                a.displayName,
                                overflow: TextOverflow.ellipsis,
                                style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
                              ),
                            ),
                            if (a.isVerified) ...[
                              const SizedBox(width: 5),
                              Icon(Icons.verified, size: 17, color: theme.colorScheme.primary),
                            ],
                          ],
                        ),
                        const SizedBox(height: 2),
                        Text.rich(
                          TextSpan(children: [
                            if (sport != null)
                              TextSpan(text: sport, style: TextStyle(color: theme.colorScheme.primary)),
                            if (sport != null && rest.isNotEmpty) const TextSpan(text: ' · '),
                            TextSpan(text: rest),
                          ]),
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(fontSize: 14, color: theme.colorScheme.onSurfaceVariant),
                        ),
                      ],
                    ),
                  ),
                  PopupMenuButton<String>(
                    icon: const Icon(Icons.more_vert),
                    tooltip: 'More',
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
                ],
              ),
            ),
            if (body != null && body.isNotEmpty)
              Padding(
                padding: const EdgeInsets.fromLTRB(0, 16, 8, 0),
                child: Text(body, style: const TextStyle(fontSize: 17, height: 1.45)),
              ),
            if (p.images.isNotEmpty)
              Padding(
                padding: const EdgeInsets.fromLTRB(0, 16, 8, 0),
                child: ClipRRect(borderRadius: BorderRadius.circular(20), child: _Images(urls: p.images)),
              ),
            if (p.sport != null || p.eventId != null)
              Padding(
                padding: const EdgeInsets.only(top: 12, right: 8),
                child: Wrap(
                  spacing: 8,
                  children: [
                    if (p.sport != null) Chip(label: Text(p.sport!.name), visualDensity: VisualDensity.compact),
                    if (p.eventId != null)
                      ActionChip(
                        avatar: const Icon(Icons.event, size: 18),
                        label: const Text('View event'),
                        visualDensity: VisualDensity.compact,
                        onPressed: () => context.push(Routes.event(p.eventId!)),
                      ),
                  ],
                ),
              ),
            const SizedBox(height: 14),
            Row(
              children: [
                _Action(
                  icon: p.likedByMe ? Icons.favorite : Icons.favorite_border,
                  color: p.likedByMe ? const Color(0xFFF43F5E) : null,
                  count: p.likeCount,
                  tooltip: p.likedByMe ? 'Unlike' : 'Like',
                  onTap: _liking ? null : _toggleLike,
                ),
                const SizedBox(width: 10),
                _Action(
                  icon: Icons.chat_bubble_outline,
                  count: p.commentCount,
                  tooltip: 'Comments',
                  onTap: widget.openOnTap ? () => context.push(Routes.post(p.id)) : null,
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

/// Round outlined action button; shows the count next to the icon when there is one.
class _Action extends StatelessWidget {
  const _Action({required this.icon, required this.count, required this.tooltip, this.onTap, this.color});

  final IconData icon;
  final int count;
  final String tooltip;
  final VoidCallback? onTap;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Tooltip(
      message: tooltip,
      child: Material(
        color: scheme.surfaceContainer,
        shape: StadiumBorder(side: BorderSide(color: scheme.outline)),
        clipBehavior: Clip.antiAlias,
        child: InkWell(
          onTap: onTap,
          child: Container(
            constraints: const BoxConstraints(minWidth: 64),
            height: 52,
            padding: const EdgeInsets.symmetric(horizontal: 18),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(icon, size: 24, color: color ?? scheme.onSurface),
                if (count > 0) ...[
                  const SizedBox(width: 8),
                  Text('$count', style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
                ],
              ],
            ),
          ),
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

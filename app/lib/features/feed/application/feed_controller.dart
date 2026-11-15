import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/media/media_upload.dart';
import '../data/feed_api.dart';
import '../data/post_models.dart';

final postProvider = FutureProvider.autoDispose.family<Post, int>(
  (ref, id) => ref.watch(feedApiProvider).get(id),
);

/// Writes for the feed. Lists patch themselves with the returned values;
/// the cached single post is invalidated.
class FeedController {
  FeedController(this._ref);

  final Ref _ref;

  FeedApi get _api => _ref.read(feedApiProvider);

  /// Like or unlike; returns the post with the server's count.
  Future<Post> toggleLike(Post post) async {
    final r = post.likedByMe ? await _api.unlike(post.id) : await _api.like(post.id);
    _ref.invalidate(postProvider(post.id));
    return post.copyWith(likedByMe: r.liked, likeCount: r.likeCount);
  }

  /// Uploads each image (`post_image`), then creates the post. The app only
  /// ever handles upload ids, never storage keys.
  Future<Post> create({
    String? body,
    List<UploadFile> images = const [],
    int? sportId,
    int? eventId,
    String? visibility,
  }) async {
    final uploader = _ref.read(mediaUploaderProvider);
    final ids = <int>[];
    for (final f in images) {
      ids.add(await uploader.upload(MediaPurpose.postImage, f));
    }
    return _api.create(PostInput(
      body: body,
      mediaUploadIds: ids,
      sportId: sportId,
      eventId: eventId,
      visibility: visibility,
    ));
  }

  Future<void> delete(int postId) async {
    await _api.delete(postId);
    _ref.invalidate(postProvider(postId));
  }

  Future<Comment> addComment(int postId, String body) async {
    final c = await _api.addComment(postId, body);
    _ref.invalidate(postProvider(postId));
    return c;
  }

  Future<void> deleteComment(int postId, int commentId) async {
    await _api.deleteComment(commentId);
    _ref.invalidate(postProvider(postId));
  }
}

final feedControllerProvider = Provider<FeedController>(FeedController.new);

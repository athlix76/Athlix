import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/envelope.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import 'post_models.dart';

class FeedApi {
  const FeedApi(this._api);

  final ApiClient _api;

  Future<Paged<Post>> feed(String mode, {String? cursor}) =>
      _api.page('/feed', Post.fromJson, query: {'mode': mode, 'cursor': cursor});

  Future<Paged<Post>> userPosts(int userId, {String? cursor}) =>
      _api.page('/users/$userId/posts', Post.fromJson, query: {'cursor': cursor});

  Future<Post> create(PostInput input) =>
      _api.post('/posts', (d) => Post.fromJson(asMap(d)), body: input.toJson());

  Future<Post> get(int id) => _api.get('/posts/$id', (d) => Post.fromJson(asMap(d)));

  Future<void> delete(int id) => _api.delete('/posts/$id');

  Future<LikeResult> like(int id) =>
      _api.post('/posts/$id/like', (d) => LikeResult.fromJson(asMap(d)));

  Future<LikeResult> unlike(int id) =>
      _api.deleteJson('/posts/$id/like', (d) => LikeResult.fromJson(asMap(d)));

  /// Oldest first.
  Future<Paged<Comment>> comments(int postId, {String? cursor}) =>
      _api.page('/posts/$postId/comments', Comment.fromJson, query: {'cursor': cursor});

  Future<Comment> addComment(int postId, String body) => _api.post(
        '/posts/$postId/comments',
        (d) => Comment.fromJson(asMap(d)),
        body: {'body': body},
      );

  Future<void> deleteComment(int id) => _api.delete('/comments/$id');
}

final feedApiProvider = Provider<FeedApi>((ref) => FeedApi(ref.watch(apiClientProvider)));

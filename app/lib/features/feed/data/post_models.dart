import '../../../core/json/json_utils.dart';
import '../../../core/models/sport.dart';
import '../../../core/models/user_summary.dart';

/// `mode` of `GET /feed`.
abstract final class FeedMode {
  static const following = 'following';
  static const discover = 'discover';
}

/// `visibility` of a post.
abstract final class PostVisibility {
  static const public = 'public';
  static const followers = 'followers';

  static const values = [public, followers];
  static const labels = {public: 'Everyone', followers: 'Followers only'};
}

/// `Post` (docs/api.md "Feed"). `images` are short-lived signed URLs: never
/// cache them across sessions.
class Post {
  const Post({
    required this.id,
    required this.author,
    required this.body,
    required this.images,
    required this.sport,
    required this.eventId,
    required this.visibility,
    required this.likeCount,
    required this.commentCount,
    required this.likedByMe,
    required this.isOwn,
    required this.createdAt,
  });

  final int id;
  final UserSummary author;
  final String? body;
  final List<String> images;
  final SportRef? sport;
  final int? eventId;
  final String visibility;
  final int likeCount;
  final int commentCount;
  final bool likedByMe;
  final bool isOwn;
  final DateTime? createdAt;

  factory Post.fromJson(Map<String, dynamic> json) => Post(
        id: asInt(json['id']),
        author: UserSummary.fromJson(asMap(json['author'])),
        body: asStringOrNull(json['body']),
        images: asStringList(json['images']),
        sport: SportRef.maybe(json['sport']),
        eventId: asIntOrNull(json['eventId']),
        visibility: asString(json['visibility'], PostVisibility.public),
        likeCount: asInt(json['likeCount']),
        commentCount: asInt(json['commentCount']),
        likedByMe: asBool(json['likedByMe']),
        isOwn: asBool(json['isOwn']),
        createdAt: asDateTimeOrNull(json['createdAt']),
      );

  Post copyWith({bool? likedByMe, int? likeCount, int? commentCount}) => Post(
        id: id,
        author: author,
        body: body,
        images: images,
        sport: sport,
        eventId: eventId,
        visibility: visibility,
        likeCount: likeCount ?? this.likeCount,
        commentCount: commentCount ?? this.commentCount,
        likedByMe: likedByMe ?? this.likedByMe,
        isOwn: isOwn,
        createdAt: createdAt,
      );
}

/// `Comment`.
class Comment {
  const Comment({
    required this.id,
    required this.postId,
    required this.author,
    required this.body,
    required this.isOwn,
    required this.createdAt,
  });

  final int id;
  final int postId;
  final UserSummary author;
  final String body;
  final bool isOwn;
  final DateTime? createdAt;

  factory Comment.fromJson(Map<String, dynamic> json) => Comment(
        id: asInt(json['id']),
        postId: asInt(json['postId']),
        author: UserSummary.fromJson(asMap(json['author'])),
        body: asString(json['body']),
        isOwn: asBool(json['isOwn']),
        createdAt: asDateTimeOrNull(json['createdAt']),
      );
}

/// `{liked, likeCount}` from like / unlike.
class LikeResult {
  const LikeResult({required this.liked, required this.likeCount});

  final bool liked;
  final int likeCount;

  factory LikeResult.fromJson(Map<String, dynamic> json) =>
      LikeResult(liked: asBool(json['liked']), likeCount: asInt(json['likeCount']));
}

/// Body of `POST /posts`.
class PostInput {
  const PostInput({
    this.body,
    this.mediaUploadIds = const [],
    this.sportId,
    this.eventId,
    this.visibility,
  });

  final String? body;
  final List<int> mediaUploadIds;
  final int? sportId;
  final int? eventId;
  final String? visibility;

  Map<String, dynamic> toJson() => {
        if (body != null && body!.isNotEmpty) 'body': body,
        if (mediaUploadIds.isNotEmpty) 'mediaUploadIds': mediaUploadIds,
        if (sportId != null) 'sportId': sportId,
        if (eventId != null) 'eventId': eventId,
        if (visibility != null) 'visibility': visibility,
      };
}

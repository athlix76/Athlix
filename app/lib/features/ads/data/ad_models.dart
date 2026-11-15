import '../../../core/json/json_utils.dart';

/// `Ad` from docs/api.md (Ads). Targeting is contextual only.
class Ad {
  const Ad({
    required this.id,
    required this.advertiserName,
    required this.title,
    required this.body,
    required this.imageUrl,
    required this.targetUrl,
    required this.placement,
    required this.label,
  });

  final int id;
  final String advertiserName;
  final String title;
  final String? body;
  final String? imageUrl;
  final String targetUrl;
  final String placement;

  /// Disclosure text from the server, e.g. "Sponsored". Always shown.
  final String label;

  factory Ad.fromJson(Map<String, dynamic> json) => Ad(
        id: asInt(json['id']),
        advertiserName: asString(json['advertiserName']),
        title: asString(json['title']),
        body: asStringOrNull(json['body']),
        imageUrl: asStringOrNull(json['imageUrl']),
        targetUrl: asString(json['targetUrl']),
        placement: asString(json['placement']),
        label: asString(json['label'], 'Sponsored'),
      );
}

/// Key of one ad request (Dart records compare by value, so it works as a family key).
typedef AdQuery = ({String placement, int? sportId, String? state});

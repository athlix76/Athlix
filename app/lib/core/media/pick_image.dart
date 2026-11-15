import 'package:image_picker/image_picker.dart';

import 'media_upload.dart';

/// Picks one image from the gallery or camera. Returns null when cancelled.
/// Resizing keeps uploads small; it is a transport choice, not a business limit
/// (size and type limits come from `media.*` public config).
Future<UploadFile?> pickImageFile({ImageSource source = ImageSource.gallery}) async {
  final picker = ImagePicker();
  final x = await picker.pickImage(source: source, maxWidth: 2048, imageQuality: 85);
  if (x == null) return null;
  final bytes = await x.readAsBytes();
  return UploadFile(
    bytes: bytes,
    contentType: contentTypeForName(x.name, reported: x.mimeType),
    name: x.name,
  );
}

/// Picks several images (feed posts). [limit] comes from `feed.post_max_media`.
Future<List<UploadFile>> pickImageFiles({int? limit}) async {
  final picker = ImagePicker();
  final xs = await picker.pickMultiImage(maxWidth: 2048, imageQuality: 85);
  final chosen = limit == null ? xs : xs.take(limit).toList();
  final out = <UploadFile>[];
  for (final x in chosen) {
    out.add(UploadFile(
      bytes: await x.readAsBytes(),
      contentType: contentTypeForName(x.name, reported: x.mimeType),
      name: x.name,
    ));
  }
  return out;
}

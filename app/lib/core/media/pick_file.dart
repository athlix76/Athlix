import 'package:file_picker/file_picker.dart';

import 'media_upload.dart';

/// Picks one document (e.g. a location proof). [extensions] without dots,
/// e.g. `['pdf', 'jpg', 'jpeg', 'png']`. Returns null when cancelled.
/// The bytes stay in memory only; nothing is written to disk by the app.
Future<UploadFile?> pickDocumentFile({required List<String> extensions}) async {
  final result = await FilePicker.platform.pickFiles(
    type: FileType.custom,
    allowedExtensions: extensions,
    withData: true,
  );
  final f = result?.files.single;
  final bytes = f?.bytes;
  if (f == null || bytes == null) return null;
  return UploadFile(bytes: bytes, contentType: contentTypeForName(f.name), name: f.name);
}

import 'dart:typed_data';

import 'file_picker_stub.dart' if (dart.library.js_interop) 'file_picker_web.dart' as impl;

/// A file the admin picked in the browser. Bytes stay in memory only.
class PickedFile {
  const PickedFile({required this.name, required this.contentType, required this.bytes});

  final String name;
  final String contentType;
  final Uint8List bytes;
}

/// Opens the browser file dialog filtered by [accept] (e.g. `image/jpeg,image/png`).
/// Null when the admin cancels. Outside the browser (tests) it always returns null.
Future<PickedFile?> pickFile({required String accept}) => impl.pickFile(accept: accept);

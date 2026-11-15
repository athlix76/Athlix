import 'dart:async';
import 'dart:js_interop';
import 'dart:typed_data';

import 'package:web/web.dart' as web;

import 'picked_file.dart';

/// Browser implementation: a hidden `<input type="file">`.
Future<PickedFile?> pickFile({required String accept}) {
  final done = Completer<PickedFile?>();
  final input = web.HTMLInputElement()
    ..type = 'file'
    ..accept = accept;
  input.onchange = (web.Event _) {
    final files = input.files;
    if (files == null || files.length == 0) {
      if (!done.isCompleted) done.complete(null);
      return;
    }
    final file = files.item(0)!;
    final reader = web.FileReader();
    reader.onload = (web.Event _) {
      final result = reader.result;
      if (result == null) {
        if (!done.isCompleted) done.complete(null);
        return;
      }
      final bytes = (result as JSArrayBuffer).toDart.asUint8List();
      if (!done.isCompleted) {
        done.complete(PickedFile(name: file.name, contentType: file.type, bytes: Uint8List.fromList(bytes)));
      }
    }.toJS;
    reader.onerror = (web.Event _) {
      if (!done.isCompleted) done.completeError(StateError('Could not read the file.'));
    }.toJS;
    reader.readAsArrayBuffer(file);
  }.toJS;
  // Cancelling the dialog fires "cancel" in current browsers.
  input.oncancel = (web.Event _) {
    if (!done.isCompleted) done.complete(null);
  }.toJS;
  input.click();
  return done.future;
}

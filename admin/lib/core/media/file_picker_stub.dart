import 'picked_file.dart';

/// Non-web builds (unit tests): no file dialog.
Future<PickedFile?> pickFile({required String accept}) async => null;

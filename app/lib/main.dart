import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app.dart';

/// Entry point. Build-time settings come from `--dart-define`
/// (`API_BASE_URL`, `ENV`; see app/README.md). Firebase is initialised lazily
/// by the push service, so the app also runs without Firebase config files.
void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const ProviderScope(child: AthlixApp()));
}

import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

/// Calls [onIdle] after [timeout] without pointer or keyboard input.
class IdleWatcher extends StatefulWidget {
  const IdleWatcher({super.key, required this.timeout, required this.onIdle, required this.child});

  final Duration timeout;
  final VoidCallback onIdle;
  final Widget child;

  @override
  State<IdleWatcher> createState() => _IdleWatcherState();
}

class _IdleWatcherState extends State<IdleWatcher> {
  Timer? _timer;
  DateTime _lastReset = DateTime.fromMillisecondsSinceEpoch(0);

  @override
  void initState() {
    super.initState();
    HardwareKeyboard.instance.addHandler(_onKey);
    _reset(force: true);
  }

  @override
  void dispose() {
    HardwareKeyboard.instance.removeHandler(_onKey);
    _timer?.cancel();
    super.dispose();
  }

  bool _onKey(KeyEvent event) {
    _reset();
    return false; // never consume the key
  }

  void _reset({bool force = false}) {
    final now = DateTime.now();
    // Pointer moves fire constantly; restarting the timer once a second is enough.
    if (!force && now.difference(_lastReset) < const Duration(seconds: 1)) return;
    _lastReset = now;
    _timer?.cancel();
    _timer = Timer(widget.timeout, widget.onIdle);
  }

  @override
  Widget build(BuildContext context) {
    return Listener(
      behavior: HitTestBehavior.translucent,
      onPointerDown: (_) => _reset(),
      onPointerMove: (_) => _reset(),
      onPointerHover: (_) => _reset(),
      onPointerSignal: (_) => _reset(),
      child: widget.child,
    );
  }
}

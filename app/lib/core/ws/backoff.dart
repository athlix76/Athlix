import 'dart:math';

/// Exponential reconnect delay: base, 2×base, 4×base … capped at [max], with
/// optional jitter (up to +25%) so many clients do not reconnect in lock-step.
/// These are transport timings, not business numbers.
class Backoff {
  Backoff({
    this.base = const Duration(seconds: 1),
    this.max = const Duration(seconds: 60),
    this.jitter = true,
    Random? random,
  }) : _random = random ?? Random();

  final Duration base;
  final Duration max;
  final bool jitter;
  final Random _random;
  int _attempt = 0;

  int get attempt => _attempt;

  Duration next() {
    final exp = _attempt < 20 ? (1 << _attempt) : (1 << 20);
    _attempt++;
    var ms = base.inMilliseconds * exp;
    if (ms > max.inMilliseconds) ms = max.inMilliseconds;
    if (jitter) ms += _random.nextInt((ms ~/ 4) + 1);
    return Duration(milliseconds: ms);
  }

  void reset() => _attempt = 0;
}

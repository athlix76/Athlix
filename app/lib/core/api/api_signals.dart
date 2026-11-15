import 'dart:async';

/// Cross-cutting API outcomes the session / gate layers react to.
enum ApiSignalType { sessionExpired, onboardingRequired, suspended, maintenance }

class ApiSignal {
  const ApiSignal(this.type, {this.nextStep});

  final ApiSignalType type;

  /// For [ApiSignalType.onboardingRequired]: `fields.nextStep`.
  final String? nextStep;
}

/// A broadcast bus between [ApiClient] (producer) and the session controller /
/// bootstrap controller (consumers). Keeps the providers free of cycles.
class ApiSignals {
  final StreamController<ApiSignal> _controller = StreamController<ApiSignal>.broadcast();

  Stream<ApiSignal> get stream => _controller.stream;

  void emit(ApiSignal signal) {
    if (!_controller.isClosed) _controller.add(signal);
  }

  Future<void> close() => _controller.close();
}

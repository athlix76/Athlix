import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../api/api_exception.dart';
import '../router/routes.dart';

/// User-safe text for any error. API messages are safe by contract; anything
/// else gets a generic sentence (never `toString()` of an exception).
String errorMessage(Object? error) {
  if (error is ApiException) {
    if (error.code != ApiErrorCode.validationFailed || error.fields.isEmpty) return error.message;
    final lines = error.fields.entries.map((e) => '${e.key}: ${e.value}').join('\n');
    return '${error.message}\n$lines';
  }
  return 'Something went wrong. Please try again.';
}

void showErrorSnack(BuildContext context, Object error) {
  if (!context.mounted) return;
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(content: Text(errorMessage(error))));
}

void showInfoSnack(BuildContext context, String text) {
  if (!context.mounted) return;
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(content: Text(text)));
}

/// Full-area error with an optional retry. `PLAN_REQUIRED` offers premium.
class ErrorView extends StatelessWidget {
  const ErrorView({super.key, required this.error, this.onRetry});

  final Object? error;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final e = error;
    if (e is ApiException && e.code == ApiErrorCode.planRequired) {
      return PlanRequiredView(message: e.message);
    }
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              e is ApiException && e.code == ApiErrorCode.network
                  ? Icons.wifi_off
                  : Icons.error_outline,
              size: 40,
            ),
            const SizedBox(height: 12),
            Text(errorMessage(e), textAlign: TextAlign.center),
            if (onRetry != null) ...[
              const SizedBox(height: 12),
              OutlinedButton(onPressed: onRetry, child: const Text('Try again')),
            ],
          ],
        ),
      ),
    );
  }
}

/// Shown when a premium benefit is missing (403 `PLAN_REQUIRED`).
class PlanRequiredView extends StatelessWidget {
  const PlanRequiredView({super.key, this.message});

  final String? message;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.workspace_premium, size: 40),
            const SizedBox(height: 12),
            Text(message ?? 'This needs a premium plan.', textAlign: TextAlign.center),
            const SizedBox(height: 12),
            FilledButton(
              onPressed: () => context.push(Routes.premium),
              child: const Text('See plans'),
            ),
          ],
        ),
      ),
    );
  }
}

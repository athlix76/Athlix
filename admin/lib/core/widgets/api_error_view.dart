import 'package:flutter/material.dart';

import '../api/api_error.dart';

/// Inline error box: title, code, server message, hint, per-field messages and
/// request id. Used in dialogs and above forms.
class ApiErrorBanner extends StatelessWidget {
  const ApiErrorBanner({super.key, required this.error});

  final Object error;

  @override
  Widget build(BuildContext context) {
    final e = ApiError.from(error);
    final scheme = Theme.of(context).colorScheme;
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: scheme.errorContainer,
        borderRadius: BorderRadius.circular(8),
      ),
      child: DefaultTextStyle.merge(
        style: TextStyle(color: scheme.onErrorContainer),
        child: SelectionArea(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Row(children: [
                Icon(Icons.error_outline, color: scheme.onErrorContainer, size: 18),
                const SizedBox(width: 8),
                Expanded(
                  child: Text('${e.title}  ·  ${e.code}${e.status != null ? '  ·  HTTP ${e.status}' : ''}',
                      style: const TextStyle(fontWeight: FontWeight.w600)),
                ),
              ]),
              const SizedBox(height: 6),
              Text(e.message),
              if (e.hint != null) ...[
                const SizedBox(height: 4),
                Text(e.hint!, style: const TextStyle(fontStyle: FontStyle.italic)),
              ],
              if (e.fields.isNotEmpty) ...[
                const SizedBox(height: 6),
                for (final f in e.fields.entries) Text('• ${f.key}: ${f.value}'),
              ],
              if (e.requestId != null) ...[
                const SizedBox(height: 6),
                Text('Request id: ${e.requestId}', style: const TextStyle(fontSize: 12)),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

/// Full-area view for a failed load. A 403 shows [ForbiddenView].
class ApiErrorView extends StatelessWidget {
  const ApiErrorView({super.key, required this.error, this.onRetry});

  final Object error;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final e = ApiError.from(error);
    if (e.isForbidden) return const ForbiddenView();
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 560),
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            ApiErrorBanner(error: e),
            if (onRetry != null) ...[
              const SizedBox(height: 12),
              OutlinedButton.icon(onPressed: onRetry, icon: const Icon(Icons.refresh), label: const Text('Retry')),
            ],
          ]),
        ),
      ),
    );
  }
}

/// Shown when the admin lacks the permission (menu gate or a 403 from the API).
class ForbiddenView extends StatelessWidget {
  const ForbiddenView({super.key, this.permission});

  final String? permission;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          const Icon(Icons.lock_outline, size: 48),
          const SizedBox(height: 12),
          Text('You do not have access to this', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 6),
          Text(
            permission == null
                ? 'The server refused this request (403 FORBIDDEN). Ask a super admin to add the permission to your role.'
                : 'This screen needs the "$permission" permission. Ask a super admin to add it to your role.',
            textAlign: TextAlign.center,
          ),
        ]),
      ),
    );
  }
}

import 'package:flutter/material.dart';

import 'api_error_view.dart';

/// Asks for a reason inside the confirmation dialog.
class ReasonSpec {
  const ReasonSpec({this.label = 'Reason', this.required = true, this.initial, this.hint});

  final String label;
  final bool required;
  final String? initial;
  final String? hint;

  /// UI bound only; the backend has the real limit.
  static const maxLength = 500;
}

/// Every write in the panel goes through this: a confirmation dialog that runs
/// [run], keeps itself open on failure and shows the API error code, message
/// and field errors. Returns the result of [run], or null if cancelled.
Future<T?> confirmAction<T extends Object>(
  BuildContext context, {
  required String title,
  required String message,
  required Future<T> Function(String? reason) run,
  String confirmLabel = 'Confirm',
  bool destructive = false,
  ReasonSpec? reason,
  Widget? details,
  String? successMessage,
}) async {
  final result = await showDialog<T>(
    context: context,
    barrierDismissible: false,
    builder: (_) => _ConfirmActionDialog<T>(
      title: title,
      message: message,
      run: run,
      confirmLabel: confirmLabel,
      destructive: destructive,
      reason: reason,
      details: details,
    ),
  );
  if (result != null && successMessage != null && context.mounted) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(successMessage)));
  }
  return result;
}

class _ConfirmActionDialog<T extends Object> extends StatefulWidget {
  const _ConfirmActionDialog({
    required this.title,
    required this.message,
    required this.run,
    required this.confirmLabel,
    required this.destructive,
    this.reason,
    this.details,
  });

  final String title;
  final String message;
  final Future<T> Function(String? reason) run;
  final String confirmLabel;
  final bool destructive;
  final ReasonSpec? reason;
  final Widget? details;

  @override
  State<_ConfirmActionDialog<T>> createState() => _ConfirmActionDialogState<T>();
}

class _ConfirmActionDialogState<T extends Object> extends State<_ConfirmActionDialog<T>> {
  late final TextEditingController _reason = TextEditingController(text: widget.reason?.initial ?? '');
  bool _busy = false;
  Object? _error;
  String? _reasonError;

  @override
  void dispose() {
    _reason.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final spec = widget.reason;
    final text = _reason.text.trim();
    if (spec != null && spec.required && text.isEmpty) {
      setState(() => _reasonError = '${spec.label} is required');
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
      _reasonError = null;
    });
    try {
      final r = await widget.run(spec == null ? null : (text.isEmpty ? null : text));
      if (mounted) Navigator.of(context).pop(r);
    } catch (e) {
      if (mounted) {
        setState(() {
          _busy = false;
          _error = e;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return AlertDialog(
      title: Text(widget.title),
      content: SizedBox(
        width: 520,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(widget.message),
              if (widget.details != null) ...[const SizedBox(height: 12), widget.details!],
              if (widget.reason != null) ...[
                const SizedBox(height: 12),
                TextField(
                  controller: _reason,
                  enabled: !_busy,
                  maxLength: ReasonSpec.maxLength,
                  minLines: 2,
                  maxLines: 5,
                  decoration: InputDecoration(
                    labelText: widget.reason!.required ? '${widget.reason!.label} *' : widget.reason!.label,
                    hintText: widget.reason!.hint,
                    errorText: _reasonError,
                    border: const OutlineInputBorder(),
                  ),
                ),
              ],
              if (_error != null) ...[const SizedBox(height: 12), ApiErrorBanner(error: _error!)],
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _busy ? null : () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        FilledButton(
          style: widget.destructive ? FilledButton.styleFrom(backgroundColor: scheme.error) : null,
          onPressed: _busy ? null : _submit,
          child: _busy
              ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
              : Text(widget.confirmLabel),
        ),
      ],
    );
  }
}

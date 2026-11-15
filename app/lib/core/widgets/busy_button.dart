import 'package:flutter/material.dart';

import 'error_view.dart';

enum BusyButtonStyle { filled, outlined, text }

/// A button that runs an async action, shows a spinner meanwhile, and shows
/// any error as a snack bar. Pass `null` to disable it.
class BusyButton extends StatefulWidget {
  const BusyButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.style = BusyButtonStyle.filled,
    this.icon,
  });

  final String label;
  final Future<void> Function()? onPressed;
  final BusyButtonStyle style;
  final IconData? icon;

  @override
  State<BusyButton> createState() => _BusyButtonState();
}

class _BusyButtonState extends State<BusyButton> {
  bool _busy = false;

  Future<void> _run() async {
    final action = widget.onPressed;
    if (action == null || _busy) return;
    setState(() => _busy = true);
    try {
      await action();
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final onPressed = widget.onPressed == null || _busy ? null : _run;
    final Widget child = _busy
        ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
        : Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (widget.icon != null) ...[Icon(widget.icon, size: 18), const SizedBox(width: 8)],
              Flexible(child: Text(widget.label, overflow: TextOverflow.ellipsis)),
            ],
          );
    switch (widget.style) {
      case BusyButtonStyle.filled:
        return FilledButton(onPressed: onPressed, child: child);
      case BusyButtonStyle.outlined:
        return OutlinedButton(onPressed: onPressed, child: child);
      case BusyButtonStyle.text:
        return TextButton(onPressed: onPressed, child: child);
    }
  }
}

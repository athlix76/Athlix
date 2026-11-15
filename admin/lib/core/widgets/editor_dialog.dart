import 'package:flutter/material.dart';

import '../api/api_error.dart';
import 'api_error_view.dart';
import 'confirm_action.dart';

/// A form dialog for create / edit screens.
///
/// [builder] draws the fields; call the given `setState` after changing local
/// values (dropdowns, switches). On Save, [validate] runs first (field →
/// message; empty = ok), then a confirmation dialog shows [confirmMessage] and
/// optional [confirmDetails] (for example a diff) and calls [save]. The form
/// stays open with its values if the API call fails. Returns true when saved.
Future<bool> showEditorDialog(
  BuildContext context, {
  required String title,
  required Widget Function(BuildContext context, StateSetter setState) builder,
  required Future<Object?> Function() save,
  Map<String, String> Function()? validate,
  String? confirmTitle,
  required String confirmMessage,
  Widget? Function()? confirmDetails,
  String saveLabel = 'Save',
  double width = 560,
  String? successMessage = 'Saved',
}) async {
  final saved = await showDialog<bool>(
    context: context,
    barrierDismissible: false,
    builder: (dialogContext) {
      Map<String, String> errors = const {};
      return StatefulBuilder(builder: (ctx, setState) {
        Future<void> onSave() async {
          final e = validate?.call() ?? const <String, String>{};
          setState(() => errors = e);
          if (e.isNotEmpty) return;
          final ok = await confirmAction<bool>(
            ctx,
            title: confirmTitle ?? title,
            message: confirmMessage,
            details: confirmDetails?.call(),
            confirmLabel: saveLabel,
            run: (_) async {
              await save();
              return true;
            },
          );
          if (ok == true && ctx.mounted) Navigator.of(ctx).pop(true);
        }

        return AlertDialog(
          title: Text(title),
          content: SizedBox(
            width: width,
            child: SingleChildScrollView(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  builder(ctx, setState),
                  if (errors.isNotEmpty) ...[
                    const SizedBox(height: 12),
                    ApiErrorBanner(error: ApiError.invalidInput(errors)),
                  ],
                ],
              ),
            ),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.of(ctx).pop(false), child: const Text('Cancel')),
            FilledButton(onPressed: onSave, child: Text(saveLabel)),
          ],
        );
      });
    },
  );
  if (saved == true && successMessage != null && context.mounted) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(successMessage)));
  }
  return saved == true;
}

/// Spacing helper for editor forms: puts 12px between fields.
Widget formColumn(List<Widget> children) => Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        for (var i = 0; i < children.length; i++) ...[if (i > 0) const SizedBox(height: 12), children[i]],
      ],
    );

/// An outlined text field for editor forms.
class FormText extends StatelessWidget {
  const FormText({super.key, required this.label, required this.controller, this.hint, this.maxLines = 1, this.keyboardType, this.enabled = true, this.monospace = false, this.prefixText, this.suffixText});

  final String label;
  final TextEditingController controller;
  final String? hint;
  final int maxLines;
  final TextInputType? keyboardType;
  final bool enabled;
  final bool monospace;
  final String? prefixText;
  final String? suffixText;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      enabled: enabled,
      maxLines: maxLines,
      minLines: maxLines > 1 ? 3 : 1,
      keyboardType: keyboardType,
      style: monospace ? const TextStyle(fontFamily: 'monospace', fontSize: 13) : null,
      decoration: InputDecoration(
        labelText: label,
        hintText: hint,
        prefixText: prefixText,
        suffixText: suffixText,
        border: const OutlineInputBorder(),
      ),
    );
  }
}

/// A labelled dropdown for editor forms (no "Any" entry unless [nullLabel] is set).
class FormDropdown<T> extends StatelessWidget {
  const FormDropdown({super.key, required this.label, required this.value, required this.items, required this.onChanged, this.nullLabel});

  final String label;
  final T? value;
  final List<(T, String)> items;
  final ValueChanged<T?>? onChanged;
  final String? nullLabel;

  @override
  Widget build(BuildContext context) {
    // A saved value the list does not contain (a new backend enum value, or a
    // lookup list that has not loaded or is not readable) stays selectable
    // instead of failing DropdownButton's "exactly one item" assertion.
    final v = value;
    final unknown = v != null && !items.any((i) => i.$1 == v);
    return DropdownButtonFormField<T?>(
      value: v,
      isExpanded: true,
      decoration: InputDecoration(labelText: label, border: const OutlineInputBorder()),
      items: [
        if (nullLabel != null) DropdownMenuItem<T?>(value: null, child: Text(nullLabel!)),
        for (final i in items) DropdownMenuItem<T?>(value: i.$1, child: Text(i.$2)),
        if (unknown) DropdownMenuItem<T?>(value: v, child: Text('$v')),
      ],
      onChanged: onChanged,
    );
  }
}

// Note for callers: controllers created for a dialog are not disposed after
// it closes. The dialog's exit animation still renders the fields, and a
// disposed controller there throws. TextEditingController holds no outside
// resources, so leaving it to the garbage collector is safe.

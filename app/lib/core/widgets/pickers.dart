import 'package:flutter/material.dart';

/// Date then time picker. Returns local time, or null when cancelled.
Future<DateTime?> pickDateTime(BuildContext context, {DateTime? initial}) async {
  final now = DateTime.now();
  final start = (initial ?? now).toLocal();
  final date = await showDatePicker(
    context: context,
    initialDate: start,
    firstDate: DateTime(now.year - 1),
    lastDate: DateTime(now.year + 3),
  );
  if (date == null || !context.mounted) return null;
  final time = await showTimePicker(context: context, initialTime: TimeOfDay.fromDateTime(start));
  if (time == null) return null;
  return DateTime(date.year, date.month, date.day, time.hour, time.minute);
}

/// Date-only picker for `YYYY-MM-DD` fields.
Future<DateTime?> pickDate(BuildContext context, {DateTime? initial, DateTime? first, DateTime? last}) {
  final now = DateTime.now();
  return showDatePicker(
    context: context,
    initialDate: initial ?? now,
    firstDate: first ?? DateTime(now.year - 80),
    lastDate: last ?? DateTime(now.year + 3),
  );
}

/// Yes/no dialog; true only when confirmed.
Future<bool> confirm(BuildContext context, {required String title, String? body, String ok = 'OK'}) async {
  final result = await showDialog<bool>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: Text(title),
      content: body == null ? null : Text(body),
      actions: [
        TextButton(onPressed: () => Navigator.of(ctx).pop(false), child: const Text('Cancel')),
        FilledButton(onPressed: () => Navigator.of(ctx).pop(true), child: Text(ok)),
      ],
    ),
  );
  return result ?? false;
}

/// Single text input dialog (e.g. a cancel reason). Null when cancelled.
Future<String?> promptText(BuildContext context,
    {required String title, String? hint, String ok = 'OK', int? maxLength, bool required = true}) async {
  final ctrl = TextEditingController();
  final result = await showDialog<String>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: Text(title),
      content: TextField(
        controller: ctrl,
        autofocus: true,
        maxLength: maxLength,
        maxLines: 3,
        minLines: 1,
        decoration: InputDecoration(hintText: hint),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.of(ctx).pop(), child: const Text('Cancel')),
        FilledButton(
          onPressed: () {
            final v = ctrl.text.trim();
            if (required && v.isEmpty) return;
            Navigator.of(ctx).pop(v);
          },
          child: Text(ok),
        ),
      ],
    ),
  );
  ctrl.dispose();
  return result;
}

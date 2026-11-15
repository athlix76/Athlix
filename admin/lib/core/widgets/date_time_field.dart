import 'package:flutter/material.dart';

import '../format/format.dart';

/// Picks a local date + time; the value is kept as UTC.
class DateTimeField extends StatelessWidget {
  const DateTimeField({super.key, required this.label, required this.value, required this.onChanged, this.allowClear = false, this.dateOnly = false});

  final String label;
  final DateTime? value;
  final ValueChanged<DateTime?> onChanged;
  final bool allowClear;
  final bool dateOnly;

  Future<void> _pick(BuildContext context) async {
    final initial = (value ?? DateTime.now()).toLocal();
    final date = await showDatePicker(
      context: context,
      initialDate: initial,
      firstDate: DateTime(2020),
      lastDate: DateTime(2100),
    );
    if (date == null || !context.mounted) return;
    if (dateOnly) {
      onChanged(DateTime(date.year, date.month, date.day).toUtc());
      return;
    }
    final time = await showTimePicker(context: context, initialTime: TimeOfDay.fromDateTime(initial));
    if (time == null) return;
    onChanged(DateTime(date.year, date.month, date.day, time.hour, time.minute).toUtc());
  }

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: () => _pick(context),
      child: InputDecorator(
        decoration: InputDecoration(
          labelText: label,
          border: const OutlineInputBorder(),
          isDense: true,
          suffixIcon: allowClear && value != null
              ? IconButton(icon: const Icon(Icons.clear), onPressed: () => onChanged(null))
              : const Icon(Icons.calendar_today, size: 18),
        ),
        child: Text(value == null ? '—' : (dateOnly ? formatDate(value) : formatDateTime(value))),
      ),
    );
  }
}

import 'package:intl/intl.dart';

/// Display helpers. API times are UTC; the UI shows device-local time.
String formatDateTime(DateTime? d) =>
    d == null ? '' : DateFormat('d MMM y, h:mm a').format(d.toLocal());

String formatDate(DateTime? d) => d == null ? '' : DateFormat('d MMM y').format(d.toLocal());

String formatTime(DateTime? d) => d == null ? '' : DateFormat('h:mm a').format(d.toLocal());

/// "now", "5m", "3h", "2d", then a date.
String timeAgo(DateTime? d, {DateTime? now}) {
  if (d == null) return '';
  final diff = (now ?? DateTime.now()).difference(d);
  if (diff.inMinutes < 1) return 'now';
  if (diff.inHours < 1) return '${diff.inMinutes}m';
  if (diff.inDays < 1) return '${diff.inHours}h';
  if (diff.inDays < 7) return '${diff.inDays}d';
  return formatDate(d);
}

/// Parses `YYYY-MM-DD` (date-only API fields).
DateTime? parseDateOnly(String? s) {
  if (s == null || s.isEmpty) return null;
  return DateTime.tryParse(s);
}

String humanize(String code) {
  if (code.isEmpty) return code;
  final s = code.replaceAll('_', ' ');
  return s[0].toUpperCase() + s.substring(1);
}

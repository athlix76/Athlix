import '../../../core/models/sport.dart';

/// Display and input helpers for template-driven stats. Behaviour depends only on
/// `StatField.type` (`int | decimal | text | duration_ms | percent`), never on a key.

String statLabel(StatField f) {
  if (f.type == 'duration_ms') return '${f.label} (seconds)';
  final unit = f.unit;
  return unit == null || unit.isEmpty ? f.label : '${f.label} ($unit)';
}

String formatStatValue(StatField? f, Object? value) {
  if (value == null) return '';
  if (f == null) return '$value';
  switch (f.type) {
    case 'duration_ms':
      final ms = value is num ? value.toInt() : int.tryParse('$value');
      if (ms == null) return '$value';
      final minutes = ms ~/ 60000;
      final seconds = (ms % 60000) / 1000;
      final secText = seconds.toStringAsFixed(seconds == seconds.roundToDouble() ? 0 : 2);
      return minutes > 0 ? '${minutes}m ${secText}s' : '${secText}s';
    case 'percent':
      return '$value%';
    default:
      final unit = f.unit;
      return unit == null || unit.isEmpty ? '$value' : '$value $unit';
  }
}

/// Text shown in the input when editing an existing value.
String statInputText(StatField f, Object? value) {
  if (value == null) return '';
  if (f.type == 'duration_ms' && value is num) {
    final s = value / 1000;
    return s == s.roundToDouble() ? s.toInt().toString() : s.toString();
  }
  return '$value';
}

/// Parses input text into the JSON value for [f]. Returns null for empty text and
/// throws [FormatException] with a user-safe message when the text is invalid.
Object? parseStatInput(StatField f, String raw) {
  final text = raw.trim();
  if (text.isEmpty) return null;
  switch (f.type) {
    case 'int':
      final v = int.tryParse(text);
      if (v == null) throw FormatException('${f.label}: enter a whole number');
      return v;
    case 'decimal':
      final v = double.tryParse(text);
      if (v == null) throw FormatException('${f.label}: enter a number');
      return v;
    case 'percent':
      final v = double.tryParse(text);
      if (v == null || v < 0 || v > 100) throw FormatException('${f.label}: enter 0 to 100');
      return v;
    case 'duration_ms':
      final v = double.tryParse(text);
      if (v == null || v < 0) throw FormatException('${f.label}: enter seconds');
      return (v * 1000).round();
    default:
      return text;
  }
}

import 'package:flutter/material.dart';

/// Minimal Markdown for policy documents: `#` headings, `-`/`*` bullets,
/// `**bold**` lines and paragraphs. Enough for admin-written policies without
/// an extra package. Text is selectable.
class SimpleMarkdown extends StatelessWidget {
  const SimpleMarkdown(this.source, {super.key});

  final String source;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final widgets = <Widget>[];
    final paragraph = StringBuffer();

    void flush() {
      if (paragraph.isEmpty) return;
      widgets.add(Padding(
        padding: const EdgeInsets.only(bottom: 12),
        child: SelectableText(_strip(paragraph.toString().trim()), style: t.bodyMedium),
      ));
      paragraph.clear();
    }

    for (final raw in source.split('\n')) {
      final line = raw.trimRight();
      if (line.trim().isEmpty) {
        flush();
        continue;
      }
      final heading = RegExp(r'^(#{1,6})\s+(.*)$').firstMatch(line);
      if (heading != null) {
        flush();
        final level = heading.group(1)!.length;
        final style = level == 1 ? t.headlineSmall : (level == 2 ? t.titleLarge : t.titleMedium);
        widgets.add(Padding(
          padding: const EdgeInsets.only(top: 8, bottom: 8),
          child: SelectableText(_strip(heading.group(2)!), style: style),
        ));
        continue;
      }
      final bullet = RegExp(r'^\s*[-*]\s+(.*)$').firstMatch(line);
      if (bullet != null) {
        flush();
        widgets.add(Padding(
          padding: const EdgeInsets.only(left: 8, bottom: 6),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text('•  '),
              Expanded(child: SelectableText(_strip(bullet.group(1)!), style: t.bodyMedium)),
            ],
          ),
        ));
        continue;
      }
      if (paragraph.isNotEmpty) paragraph.write(' ');
      paragraph.write(line.trim());
    }
    flush();
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: widgets);
  }

  static String _strip(String s) =>
      s.replaceAll('**', '').replaceAll('__', '').replaceAll('`', '');
}

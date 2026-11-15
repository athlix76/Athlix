import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../format/line_diff.dart';

/// Page header + optional filter bar + body that fills the rest.
class PageScaffold extends StatelessWidget {
  const PageScaffold({super.key, required this.title, required this.body, this.actions = const [], this.filters, this.subtitle});

  final String title;
  final String? subtitle;
  final List<Widget> actions;
  final Widget? filters;
  final Widget body;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 20, 24, 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(children: [
            Expanded(
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text(title, style: Theme.of(context).textTheme.headlineSmall),
                if (subtitle != null) Text(subtitle!, style: Theme.of(context).textTheme.bodySmall),
              ]),
            ),
            ...actions.expand((a) => [const SizedBox(width: 8), a]),
          ]),
          if (filters != null) ...[const SizedBox(height: 12), filters!],
          const SizedBox(height: 12),
          Expanded(child: body),
        ],
      ),
    );
  }
}

/// A small coloured label for statuses.
class StatusChip extends StatelessWidget {
  const StatusChip(this.status, {super.key});

  final String status;

  static const _good = {'active', 'published', 'approved', 'paid', 'verified', 'granted', 'visible', 'done', 'actioned'};
  static const _warn = {'pending_review', 'pending_payment', 'submitted', 'pending', 'open', 'draft', 'grace', 'created', 'paused', 'partially_refunded'};
  static const _bad = {'suspended', 'removed', 'rejected', 'failed', 'expired', 'cancelled', 'refunded', 'disabled', 'deleted', 'hidden', 'revoked', 'ended'};

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final Color bg;
    if (_good.contains(status)) {
      bg = Colors.green.withAlpha(46);
    } else if (_warn.contains(status)) {
      bg = Colors.orange.withAlpha(51);
    } else if (_bad.contains(status)) {
      bg = scheme.errorContainer;
    } else {
      bg = scheme.surfaceContainerHighest;
    }
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(12)),
      child: Text(status, style: const TextStyle(fontSize: 12)),
    );
  }
}

/// Label/value rows for detail screens.
class KeyValueList extends StatelessWidget {
  const KeyValueList(this.rows, {super.key, this.labelWidth = 180});

  final List<(String, Widget)> rows;
  final double labelWidth;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (final r in rows)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 4),
            child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
              SizedBox(width: labelWidth, child: Text(r.$1, style: Theme.of(context).textTheme.bodySmall)),
              Expanded(child: r.$2),
            ]),
          ),
      ],
    );
  }
}

/// Plain text value, `—` when empty.
Widget textValue(Object? v) => SelectableText(v == null || '$v'.isEmpty ? '—' : '$v');

/// A titled card section.
class SectionCard extends StatelessWidget {
  const SectionCard({super.key, required this.title, required this.child, this.actions = const []});

  final String title;
  final Widget child;
  final List<Widget> actions;

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.only(bottom: 16),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Row(children: [
            Expanded(child: Text(title, style: Theme.of(context).textTheme.titleMedium)),
            ...actions,
          ]),
          const SizedBox(height: 8),
          child,
        ]),
      ),
    );
  }
}

/// Pretty-printed JSON in a monospace block.
class JsonBlock extends StatelessWidget {
  const JsonBlock(this.value, {super.key});

  final Object? value;

  static String pretty(Object? v) => const JsonEncoder.withIndent('  ').convert(v);

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(8),
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(6),
      ),
      child: SelectableText(pretty(value), style: const TextStyle(fontFamily: 'monospace', fontSize: 12)),
    );
  }
}

/// Shows a line diff with -/+ markers.
class DiffView extends StatelessWidget {
  const DiffView({super.key, required this.before, required this.after});

  final String before;
  final String after;

  @override
  Widget build(BuildContext context) {
    final lines = lineDiff(before, after);
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(8),
      decoration: BoxDecoration(
        border: Border.all(color: Theme.of(context).dividerColor),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          for (final l in lines)
            Container(
              color: switch (l.op) {
                DiffOp.added => Colors.green.withAlpha(38),
                DiffOp.removed => Colors.red.withAlpha(38),
                DiffOp.same => null,
              },
              child: Text(
                '${switch (l.op) { DiffOp.added => '+ ', DiffOp.removed => '- ', DiffOp.same => '  ' }}${l.text}',
                style: const TextStyle(fontFamily: 'monospace', fontSize: 12),
              ),
            ),
        ],
      ),
    );
  }
}

/// Opens a (signed, short-lived) URL in a new tab.
Future<void> openUrl(String url) async {
  final uri = Uri.tryParse(url);
  if (uri == null || !(uri.isScheme('https') || uri.isScheme('http'))) return;
  await launchUrl(uri, webOnlyWindowName: '_blank');
}

/// A dropdown with an "Any" entry (value null) for filter bars.
class FilterDropdown extends StatelessWidget {
  const FilterDropdown({super.key, required this.label, required this.value, required this.options, required this.onChanged, this.anyLabel = 'Any'});

  final String label;
  final String? value;
  final List<String> options;
  final ValueChanged<String?> onChanged;
  final String anyLabel;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 200,
      child: DropdownButtonFormField<String?>(
        value: value,
        isExpanded: true,
        decoration: InputDecoration(labelText: label, border: const OutlineInputBorder(), isDense: true),
        items: [
          DropdownMenuItem<String?>(value: null, child: Text(anyLabel)),
          for (final o in options) DropdownMenuItem<String?>(value: o, child: Text(o)),
          // Keep a selected value that is not in the list, so the dropdown never asserts.
          if (value != null && !options.contains(value)) DropdownMenuItem<String?>(value: value, child: Text(value!)),
        ],
        onChanged: onChanged,
      ),
    );
  }
}

/// A compact text field for filter bars that reports on submit.
class FilterTextField extends StatelessWidget {
  const FilterTextField({super.key, required this.label, required this.controller, required this.onSubmitted, this.width = 240});

  final String label;
  final TextEditingController controller;
  final ValueChanged<String> onSubmitted;
  final double width;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: width,
      child: TextField(
        controller: controller,
        decoration: InputDecoration(
          labelText: label,
          border: const OutlineInputBorder(),
          isDense: true,
          suffixIcon: IconButton(icon: const Icon(Icons.search), onPressed: () => onSubmitted(controller.text)),
        ),
        onSubmitted: onSubmitted,
      ),
    );
  }
}

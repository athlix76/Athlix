import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../config/config_providers.dart';
import '../models/sport.dart';

/// Sport dropdown fed by `bootstrap.sports` (admin-managed catalogue).
class SportDropdown extends ConsumerWidget {
  const SportDropdown({
    super.key,
    required this.value,
    required this.onChanged,
    this.label = 'Sport',
    this.allowAny = false,
  });

  final int? value;
  final ValueChanged<int?> onChanged;
  final String label;

  /// Adds an "Any" entry (filters).
  final bool allowAny;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final sports = ref.watch(sportsProvider);
    final known = sports.any((s) => s.id == value);
    return DropdownButtonFormField<int?>(
      value: known ? value : null,
      isExpanded: true,
      decoration: InputDecoration(labelText: label),
      items: [
        if (allowAny) const DropdownMenuItem<int?>(value: null, child: Text('Any')),
        for (final s in sports) DropdownMenuItem<int?>(value: s.id, child: Text(s.name)),
      ],
      onChanged: onChanged,
    );
  }
}

/// Position dropdown for one sport, filtered by role (`appliesTo`).
class PositionDropdown extends ConsumerWidget {
  const PositionDropdown({
    super.key,
    required this.sportId,
    required this.value,
    required this.onChanged,
    this.role,
    this.allowAny = false,
  });

  final int? sportId;
  final int? value;
  final ValueChanged<int?> onChanged;
  final String? role;
  final bool allowAny;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final sport = ref.watch(bootstrapProvider).valueOrNull?.sportById(sportId);
    final positions = <Position>[
      for (final p in sport?.positions ?? const <Position>[])
        if (p.appliesToRole(role)) p,
    ];
    if (positions.isEmpty) return const SizedBox.shrink();
    final known = positions.any((p) => p.id == value);
    return DropdownButtonFormField<int?>(
      value: known ? value : null,
      isExpanded: true,
      decoration: const InputDecoration(labelText: 'Position'),
      items: [
        DropdownMenuItem<int?>(value: null, child: Text(allowAny ? 'Any' : 'None')),
        for (final p in positions) DropdownMenuItem<int?>(value: p.id, child: Text(p.name)),
      ],
      onChanged: onChanged,
    );
  }
}

/// Dropdown over a fixed set of contract enum values (`"tournament"`, ...).
class EnumDropdown extends StatelessWidget {
  const EnumDropdown({
    super.key,
    required this.label,
    required this.values,
    required this.value,
    required this.onChanged,
    this.labels = const {},
    this.allowAny = false,
  });

  final String label;
  final List<String> values;
  final String? value;
  final ValueChanged<String?> onChanged;
  final Map<String, String> labels;
  final bool allowAny;

  static String pretty(String v) {
    if (v.isEmpty) return v;
    final s = v.replaceAll('_', ' ');
    return s[0].toUpperCase() + s.substring(1);
  }

  @override
  Widget build(BuildContext context) {
    return DropdownButtonFormField<String?>(
      value: values.contains(value) ? value : null,
      isExpanded: true,
      decoration: InputDecoration(labelText: label),
      items: [
        if (allowAny) const DropdownMenuItem<String?>(value: null, child: Text('Any')),
        for (final v in values)
          DropdownMenuItem<String?>(value: v, child: Text(labels[v] ?? pretty(v))),
      ],
      onChanged: onChanged,
    );
  }
}

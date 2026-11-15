import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../application/location_identity_controller.dart';

/// State picker backed by `GET /geo/states`.
class StateDropdown extends ConsumerWidget {
  const StateDropdown({super.key, required this.value, required this.onChanged, this.enabled = true});

  final int? value;
  final ValueChanged<int?> onChanged;
  final bool enabled;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final states = ref.watch(geoStatesProvider);
    final items = states.valueOrNull ?? const [];
    return DropdownButtonFormField<int>(
      value: items.any((s) => s.id == value) ? value : null,
      isExpanded: true,
      decoration: InputDecoration(
        labelText: 'State',
        helperText: states.hasError ? 'Could not load states. Pull to retry.' : null,
        suffixIcon: states.isLoading ? const _Spinner() : null,
      ),
      items: [for (final s in items) DropdownMenuItem(value: s.id, child: Text(s.name))],
      onChanged: enabled ? onChanged : null,
    );
  }
}

/// District picker for one state (`GET /geo/districts?stateId=`).
class DistrictDropdown extends ConsumerWidget {
  const DistrictDropdown({
    super.key,
    required this.stateId,
    required this.value,
    required this.onChanged,
    this.enabled = true,
  });

  final int? stateId;
  final int? value;
  final ValueChanged<int?> onChanged;
  final bool enabled;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final id = stateId;
    if (id == null) {
      return DropdownButtonFormField<int>(
        items: const [],
        onChanged: null,
        decoration: const InputDecoration(labelText: 'District', helperText: 'Pick a state first'),
      );
    }
    final districts = ref.watch(geoDistrictsProvider(id));
    final items = districts.valueOrNull ?? const [];
    return DropdownButtonFormField<int>(
      key: ValueKey('districts-$id'),
      value: items.any((d) => d.id == value) ? value : null,
      isExpanded: true,
      decoration: InputDecoration(
        labelText: 'District',
        helperText: districts.hasError ? 'Could not load districts.' : null,
        suffixIcon: districts.isLoading ? const _Spinner() : null,
      ),
      items: [for (final d in items) DropdownMenuItem(value: d.id, child: Text(d.name))],
      onChanged: enabled ? onChanged : null,
    );
  }
}

class _Spinner extends StatelessWidget {
  const _Spinner();

  @override
  Widget build(BuildContext context) => const Padding(
        padding: EdgeInsets.all(12),
        child: SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)),
      );
}

import 'package:flutter/material.dart';

import '../../../core/format/dates.dart';
import '../../../core/money/money.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/sport_picker.dart';
import '../../location_identity/data/eligibility_models.dart';
import '../data/event_models.dart';

/// Bottom sheet for `GET /events` filters. Returns the new filters or null.
Future<EventFilters?> showEventFiltersSheet(BuildContext context, EventFilters current) {
  return showModalBottomSheet<EventFilters>(
    context: context,
    isScrollControlled: true,
    builder: (_) => _FiltersSheet(initial: current),
  );
}

class _FiltersSheet extends StatefulWidget {
  const _FiltersSheet({required this.initial});

  final EventFilters initial;

  @override
  State<_FiltersSheet> createState() => _FiltersSheetState();
}

class _FiltersSheetState extends State<_FiltersSheet> {
  late int? _sportId = widget.initial.sportId;
  late String? _eventType = widget.initial.eventType;
  late String? _level = widget.initial.competitionLevel;
  late String? _gender = widget.initial.genderCategory;
  late final _state = TextEditingController(text: widget.initial.state ?? '');
  late final _district = TextEditingController(text: widget.initial.district ?? '');
  late final _city = TextEditingController(text: widget.initial.city ?? '');
  late final _maxFee = TextEditingController(
    text: widget.initial.maxEntryFeePaise == null ? '' : paiseToRupeeInput(widget.initial.maxEntryFeePaise!),
  );
  late DateTime? _from = widget.initial.fromDate;
  late DateTime? _to = widget.initial.toDate;
  late bool _eligible = widget.initial.eligibleForMe;
  late bool _closed = widget.initial.includeClosed;
  late String? _category = widget.initial.category;

  @override
  void dispose() {
    _state.dispose();
    _district.dispose();
    _city.dispose();
    _maxFee.dispose();
    super.dispose();
  }

  String? _t(TextEditingController c) => c.text.trim().isEmpty ? null : c.text.trim();

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
      child: SafeArea(
        child: ListView(
          shrinkWrap: true,
          padding: const EdgeInsets.all(16),
          children: [
            Text('Filters', style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 12),
            SportDropdown(value: _sportId, allowAny: true, onChanged: (v) => setState(() => _sportId = v)),
            const SizedBox(height: 12),
            EnumDropdown(
              label: 'Event type',
              values: EventEnums.eventTypes,
              value: _eventType,
              allowAny: true,
              onChanged: (v) => setState(() => _eventType = v),
            ),
            const SizedBox(height: 12),
            EnumDropdown(
              label: 'Level',
              values: EventEnums.competitionLevels,
              value: _level,
              allowAny: true,
              onChanged: (v) => setState(() => _level = v),
            ),
            const SizedBox(height: 12),
            EnumDropdown(
              label: 'Category',
              values: EventEnums.genderCategories,
              value: _gender,
              allowAny: true,
              onChanged: (v) => setState(() => _gender = v),
            ),
            const SizedBox(height: 12),
            TextField(controller: _state, decoration: const InputDecoration(labelText: 'State')),
            const SizedBox(height: 12),
            TextField(controller: _district, decoration: const InputDecoration(labelText: 'District')),
            const SizedBox(height: 12),
            TextField(controller: _city, decoration: const InputDecoration(labelText: 'City')),
            const SizedBox(height: 12),
            TextField(
              controller: _maxFee,
              keyboardType: const TextInputType.numberWithOptions(decimal: true),
              decoration: const InputDecoration(labelText: 'Max entry fee', prefixText: '₹ '),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton(
                    onPressed: () async {
                      final d = await pickDate(context, initial: _from);
                      if (d != null) setState(() => _from = d);
                    },
                    child: Text(_from == null ? 'From date' : formatDate(_from)),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: OutlinedButton(
                    onPressed: () async {
                      final d = await pickDate(context, initial: _to);
                      if (d != null) setState(() => _to = d);
                    },
                    child: Text(_to == null ? 'To date' : formatDate(_to)),
                  ),
                ),
              ],
            ),
            EnumDropdown(
              label: 'Organised by',
              values: EventCategory.all,
              value: _category,
              labels: const {EventCategory.government: 'Government', EventCategory.private: 'Private'},
              allowAny: true,
              onChanged: (v) => setState(() => _category = v),
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('Only events my age fits'),
              value: _eligible,
              onChanged: (v) => setState(() => _eligible = v),
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('Include closed registrations'),
              value: _closed,
              onChanged: (v) => setState(() => _closed = v),
            ),
            const SizedBox(height: 8),
            Row(
              children: [
                TextButton(
                  onPressed: () => Navigator.pop(
                    context,
                    EventFilters(
                      q: widget.initial.q,
                      showAll: widget.initial.showAll,
                      near: widget.initial.near,
                      radiusKm: widget.initial.radiusKm,
                    ),
                  ),
                  child: const Text('Clear'),
                ),
                const Spacer(),
                FilledButton(
                  onPressed: () => Navigator.pop(
                    context,
                    EventFilters(
                      q: widget.initial.q,
                      sportId: _sportId,
                      eventType: _eventType,
                      competitionLevel: _level,
                      genderCategory: _gender,
                      state: _t(_state),
                      district: _t(_district),
                      city: _t(_city),
                      fromDate: _from,
                      toDate: _to,
                      maxEntryFeePaise: parseRupeesToPaise(_maxFee.text),
                      eligibleForMe: _eligible,
                      includeClosed: _closed,
                      category: _category,
                      showAll: widget.initial.showAll,
                      near: widget.initial.near,
                      radiusKm: widget.initial.radiusKm,
                    ),
                  ),
                  child: const Text('Apply'),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

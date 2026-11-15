import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/date_time_field.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../../sports/application/sports_providers.dart';
import '../../sports/data/sports_api.dart';
import '../application/fee_formula.dart';
import '../application/fee_rules_providers.dart';
import '../data/fee_rules_api.dart';
import 'fee_preview_card.dart';

/// Listing fee rules. The most specific active rule wins:
/// (sport + type) > sport > type > default, ties by priority.
class FeeRulesScreen extends ConsumerWidget {
  const FeeRulesScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(feeRulesProvider);
    final sports = ref.watch(adminSportsProvider).valueOrNull ?? const <AdminSport>[];
    final canWrite = ref.watch(sessionProvider).can(Perm.feesWrite);
    void reload() => ref.invalidate(feeRulesProvider);
    String sportName(int? id) {
      if (id == null) return 'any sport';
      for (final s in sports) {
        if (s.id == id) return s.name;
      }
      return 'sport #$id';
    }

    return PageScaffold(
      title: 'Listing fee rules',
      subtitle: 'Changes apply to new quotes only. Orders already created keep their quote.',
      actions: [
        IconButton(tooltip: 'Reload', icon: const Icon(Icons.refresh), onPressed: reload),
        if (canWrite)
          FilledButton.icon(
            onPressed: () async {
              if (await editFeeRule(context, ref, null, sports)) reload();
            },
            icon: const Icon(Icons.add),
            label: const Text('Add rule'),
          ),
      ],
      body: ListView(children: [
        FeePreviewCard(sports: sports),
        AsyncBody<List<FeeRule>>(
          value: value,
          onRetry: reload,
          data: (rules) => Column(children: [
            if (rules.isEmpty) const Padding(padding: EdgeInsets.all(16), child: Text('No fee rules. Listing will fail until one exists.')),
            for (final r in rules)
              Card(
                child: ListTile(
                  title: Text('${r.name}  ·  ${sportName(r.sportId)}  ·  ${r.eventType ?? 'any type'}  ·  priority ${r.priority}'),
                  subtitle: Text([
                    '${formatBp(r.percentBp)} of gross entry',
                    'min ${formatPaise(r.minFeePaise)}',
                    'max ${r.maxFeePaise == null ? 'no cap' : formatPaise(r.maxFeePaise)}',
                    'free event ${formatPaise(r.freeEventFeePaise)}',
                    'GST ${formatBp(r.gstBp)}',
                    'from ${formatDateTime(r.effectiveFrom)}',
                    'to ${r.effectiveTo == null ? 'open' : formatDateTime(r.effectiveTo)}',
                  ].join('  ·  ')),
                  leading: StatusChip(r.active ? 'active' : 'disabled'),
                  trailing: canWrite
                      ? IconButton(
                          tooltip: 'Edit',
                          icon: const Icon(Icons.edit_outlined),
                          onPressed: () async {
                            if (await editFeeRule(context, ref, r, sports)) reload();
                          },
                        )
                      : null,
                ),
              ),
          ]),
        ),
      ]),
    );
  }
}

String _ruleText(FeeRule r) => [
      'name: ${r.name}',
      'sportId: ${r.sportId ?? 'any'}',
      'eventType: ${r.eventType ?? 'any'}',
      'percent: ${formatBp(r.percentBp)}',
      'min fee: ${formatPaise(r.minFeePaise)}',
      'max fee: ${r.maxFeePaise == null ? 'no cap' : formatPaise(r.maxFeePaise)}',
      'free event fee: ${formatPaise(r.freeEventFeePaise)}',
      'GST: ${formatBp(r.gstBp)}',
      'priority: ${r.priority}',
      'active: ${r.active}',
      'effective from: ${formatDateTime(r.effectiveFrom)}',
      'effective to: ${r.effectiveTo == null ? 'open' : formatDateTime(r.effectiveTo)}',
    ].join('\n');

Future<bool> editFeeRule(BuildContext context, WidgetRef ref, FeeRule? r, List<AdminSport> sports) async {
  final api = ref.read(feeRulesApiProvider);
  final name = TextEditingController(text: r?.name ?? '');
  final percent = TextEditingController(text: bpToInput(r?.percentBp));
  final minFee = TextEditingController(text: paiseToInput(r?.minFeePaise));
  final maxFee = TextEditingController(text: paiseToInput(r?.maxFeePaise));
  final freeFee = TextEditingController(text: paiseToInput(r?.freeEventFeePaise));
  final gst = TextEditingController(text: bpToInput(r?.gstBp));
  final priority = TextEditingController(text: '${r?.priority ?? 0}');
  final sampleFee = TextEditingController();
  final sampleSlots = TextEditingController();
  int? sportId = r?.sportId;
  String? eventType = r?.eventType;
  var active = r?.active ?? true;
  DateTime? from = r?.effectiveFrom;
  DateTime? to = r?.effectiveTo;

  Map<String, String> errors() {
    final e = <String, String>{};
    if (name.text.trim().isEmpty) e['name'] = 'is required';
    if (parsePercentToBp(percent.text) == null) e['percent'] = 'a percentage from 0 to 100, up to 2 decimals';
    if (parseRupeesToPaise(minFee.text) == null) e['min fee'] = 'an amount in rupees';
    if (maxFee.text.trim().isNotEmpty) {
      final max = parseRupeesToPaise(maxFee.text);
      final min = parseRupeesToPaise(minFee.text);
      if (max == null) {
        e['max fee'] = 'an amount in rupees, or empty for no cap';
      } else if (min != null && max < min) {
        e['max fee'] = 'must not be below the min fee';
      }
    }
    if (parseRupeesToPaise(freeFee.text) == null) e['free event fee'] = 'an amount in rupees (0 allowed)';
    if (parsePercentToBp(gst.text) == null) e['GST'] = 'a percentage from 0 to 100';
    if (int.tryParse(priority.text.trim()) == null) e['priority'] = 'a whole number';
    if (from != null && to != null && !to!.isAfter(from!)) e['effective to'] = 'must be after effective from';
    return e;
  }

  FeeRule build() => FeeRule(
        id: r?.id,
        name: name.text.trim(),
        sportId: sportId,
        eventType: eventType,
        percentBp: parsePercentToBp(percent.text) ?? 0,
        minFeePaise: parseRupeesToPaise(minFee.text) ?? 0,
        maxFeePaise: maxFee.text.trim().isEmpty ? null : parseRupeesToPaise(maxFee.text),
        freeEventFeePaise: parseRupeesToPaise(freeFee.text) ?? 0,
        gstBp: parsePercentToBp(gst.text) ?? 0,
        priority: int.tryParse(priority.text.trim()) ?? 0,
        active: active,
        effectiveFrom: from,
        effectiveTo: to,
      );

  Widget localPreview() {
    final fee = parseRupeesToPaise(sampleFee.text);
    final slots = int.tryParse(sampleSlots.text.trim());
    if (fee == null || slots == null || slots <= 0 || errors().isNotEmpty) {
      return const Text('Fill the rule and a sample entry fee and slots to see the fee.');
    }
    final b = computeListingFee(build(), entryFeePaise: fee, maxSlots: slots);
    return KeyValueList([
      ('Gross entry', textValue(b.isFreeEvent ? 'free event' : formatPaise(b.grossEntryPaise))),
      if (!b.isFreeEvent) ('Percent fee', textValue(formatPaise(b.rawPercentFeePaise))),
      ('Base fee', textValue('${formatPaise(b.baseFeePaise)}${b.clampedTo != null ? '  (raised/capped to ${b.clampedTo})' : ''}')),
      ('GST', textValue(formatPaise(b.gstPaise))),
      ('Total', textValue(formatPaise(b.totalPaise))),
    ]);
  }

  return showEditorDialog(
    context,
    title: r == null ? 'Add fee rule' : 'Edit fee rule ${r.name}',
    width: 680,
    builder: (ctx, setState) => formColumn([
      FormText(label: 'Name', controller: name),
      Row(children: [
        Expanded(
          child: FormDropdown<int>(
            label: 'Sport',
            value: sportId,
            nullLabel: 'Any sport',
            items: [for (final s in sports) (s.id, s.name)],
            onChanged: (v) => setState(() => sportId = v),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: FormDropdown<String>(
            label: 'Event type',
            value: eventType,
            nullLabel: 'Any type',
            items: [for (final t in eventTypes) (t, t)],
            onChanged: (v) => setState(() => eventType = v),
          ),
        ),
      ]),
      Row(children: [
        Expanded(child: FormText(label: 'Percent of gross entry', controller: percent, suffixText: '%')),
        const SizedBox(width: 12),
        Expanded(child: FormText(label: 'GST', controller: gst, suffixText: '%')),
      ]),
      Row(children: [
        Expanded(child: FormText(label: 'Min fee', controller: minFee, prefixText: '₹ ')),
        const SizedBox(width: 12),
        Expanded(child: FormText(label: 'Max fee (empty = no cap)', controller: maxFee, prefixText: '₹ ')),
      ]),
      Row(children: [
        Expanded(child: FormText(label: 'Fee for a free event', controller: freeFee, prefixText: '₹ ')),
        const SizedBox(width: 12),
        Expanded(child: FormText(label: 'Priority (higher wins ties)', controller: priority)),
      ]),
      Row(children: [
        Expanded(child: DateTimeField(label: 'Effective from', value: from, onChanged: (v) => setState(() => from = v))),
        const SizedBox(width: 12),
        Expanded(child: DateTimeField(label: 'Effective to', value: to, allowClear: true, onChanged: (v) => setState(() => to = v))),
      ]),
      SwitchListTile(title: const Text('Active'), value: active, onChanged: (v) => setState(() => active = v)),
      const Divider(),
      Text('Try this rule (display only, not saved)', style: Theme.of(ctx).textTheme.titleSmall),
      Row(children: [
        Expanded(child: FormText(label: 'Sample entry fee', controller: sampleFee, prefixText: '₹ ')),
        const SizedBox(width: 12),
        Expanded(child: FormText(label: 'Sample max slots', controller: sampleSlots)),
        const SizedBox(width: 12),
        OutlinedButton(onPressed: () => setState(() {}), child: const Text('Calculate')),
      ]),
      localPreview(),
    ]),
    validate: errors,
    confirmTitle: r == null ? 'Create fee rule?' : 'Save fee rule?',
    confirmMessage: 'New listing quotes will use this rule straight away. Existing orders keep their quote.',
    confirmDetails: () => DiffView(before: r == null ? '' : _ruleText(r), after: _ruleText(build())),
    save: () => r == null ? api.create(build()) : api.update(r.id!, build()),
  );
}

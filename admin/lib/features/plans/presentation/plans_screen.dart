import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../application/plans_providers.dart';
import '../data/plans_api.dart';

final _codeRe = RegExp(r'^[a-z][a-z0-9_]{1,63}$');

class PlansScreen extends ConsumerWidget {
  const PlansScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(plansProvider);
    final canWrite = ref.watch(sessionProvider).can(Perm.plansWrite);
    void reload() => ref.invalidate(plansProvider);
    return PageScaffold(
      title: 'Premium plans',
      subtitle: 'Prices here are for display. Store prices are set in Google Play and App Store and must match.',
      actions: [
        IconButton(tooltip: 'Reload', icon: const Icon(Icons.refresh), onPressed: reload),
        if (canWrite)
          FilledButton.icon(
            onPressed: () async {
              if (await _editPlan(context, ref, null)) reload();
            },
            icon: const Icon(Icons.add),
            label: const Text('Add plan'),
          ),
      ],
      body: AsyncBody<List<Plan>>(
        value: value,
        onRetry: reload,
        data: (plans) => ListView(children: [
          if (plans.isEmpty) const Padding(padding: EdgeInsets.all(16), child: Text('No plans yet.')),
          for (final p in plans)
            SectionCard(
              title: '${p.name}  (${p.code})',
              actions: [
                StatusChip(p.active ? 'active' : 'disabled'),
                if (canWrite) ...[
                  const SizedBox(width: 8),
                  OutlinedButton(
                    onPressed: () async {
                      if (await _editPlan(context, ref, p)) reload();
                    },
                    child: const Text('Edit plan'),
                  ),
                  const SizedBox(width: 8),
                  OutlinedButton(
                    onPressed: () async {
                      if (await _editBenefits(context, ref, p)) reload();
                    },
                    child: const Text('Edit benefits'),
                  ),
                ],
              ],
              child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                KeyValueList([
                  ('Audience', textValue(p.audience)),
                  ('Price', textValue('${formatPaise(p.pricePaise)} / ${p.billingPeriod}')),
                  ('Play product id', textValue(p.storeProductIdAndroid)),
                  ('App Store product id', textValue(p.storeProductIdIos)),
                  ('Sort order', textValue(p.sortOrder)),
                  ('Description', textValue(p.description)),
                ]),
                const SizedBox(height: 8),
                Text('Benefits', style: Theme.of(context).textTheme.titleSmall),
                if (p.benefits.isEmpty) const Text('None'),
                for (final b in p.benefits) Text('• ${b.label}  (${b.key} = ${b.value})'),
              ]),
            ),
        ]),
      ),
    );
  }
}

String _planText(Plan p) => [
      'code: ${p.code}',
      'audience: ${p.audience}',
      'name: ${p.name}',
      'description: ${p.description}',
      'price: ${formatPaise(p.pricePaise)}',
      'billing period: ${p.billingPeriod}',
      'Play product id: ${p.storeProductIdAndroid ?? '-'}',
      'App Store product id: ${p.storeProductIdIos ?? '-'}',
      'active: ${p.active}',
      'sort order: ${p.sortOrder}',
    ].join('\n');

Future<bool> _editPlan(BuildContext context, WidgetRef ref, Plan? p) async {
  final api = ref.read(plansApiProvider);
  final code = TextEditingController(text: p?.code ?? '');
  final name = TextEditingController(text: p?.name ?? '');
  final description = TextEditingController(text: p?.description ?? '');
  final price = TextEditingController(text: paiseToInput(p?.pricePaise));
  final android = TextEditingController(text: p?.storeProductIdAndroid ?? '');
  final ios = TextEditingController(text: p?.storeProductIdIos ?? '');
  final order = TextEditingController(text: '${p?.sortOrder ?? 0}');
  var audience = p?.audience ?? 'player';
  var period = p?.billingPeriod ?? 'monthly';
  var active = p?.active ?? true;
  String? orNull(TextEditingController c) => c.text.trim().isEmpty ? null : c.text.trim();

  Plan build() => Plan(
        id: p?.id,
        code: code.text.trim(),
        audience: audience,
        name: name.text.trim(),
        description: description.text.trim(),
        pricePaise: parseRupeesToPaise(price.text) ?? 0,
        billingPeriod: period,
        storeProductIdAndroid: orNull(android),
        storeProductIdIos: orNull(ios),
        active: active,
        sortOrder: int.tryParse(order.text.trim()) ?? 0,
      );

  return showEditorDialog(
    context,
    title: p == null ? 'Add plan' : 'Edit ${p.name}',
    builder: (_, setState) => formColumn([
      FormText(label: 'Code (cannot change later)', controller: code, enabled: p == null),
      FormText(label: 'Name', controller: name),
      FormText(label: 'Description', controller: description, maxLines: 4),
      Row(children: [
        Expanded(
          child: FormDropdown<String>(
            label: 'Audience',
            value: audience,
            items: [for (final a in planAudiences) (a, a)],
            onChanged: (v) => setState(() => audience = v ?? audience),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: FormDropdown<String>(
            label: 'Billing period',
            value: period,
            items: [for (final b in billingPeriods) (b, b)],
            onChanged: (v) => setState(() => period = v ?? period),
          ),
        ),
      ]),
      FormText(label: 'Display price', controller: price, prefixText: '₹ '),
      FormText(label: 'Google Play product id', controller: android),
      FormText(label: 'App Store product id', controller: ios),
      FormText(label: 'Sort order', controller: order),
      SwitchListTile(title: const Text('Active (can be bought)'), value: active, onChanged: (v) => setState(() => active = v)),
    ]),
    validate: () => {
      if (p == null && !_codeRe.hasMatch(code.text.trim())) 'code': 'lower case letters, digits and _',
      if (name.text.trim().isEmpty) 'name': 'is required',
      if (parseRupeesToPaise(price.text) == null) 'price': 'an amount in rupees',
      if (int.tryParse(order.text.trim()) == null) 'sort order': 'a whole number',
    },
    confirmMessage: p == null ? 'Create this plan?' : 'Save the plan? Existing subscribers keep their subscription.',
    confirmDetails: () => DiffView(before: p == null ? '' : _planText(p), after: _planText(build())),
    save: () => p == null ? api.create(build()) : api.update(p.id!, build()),
  );
}

class _BenefitRow {
  _BenefitRow(this.def, PlanBenefit? existing)
      : included = existing != null,
        flag = existing?.value == true,
        count = TextEditingController(text: existing == null ? '' : '${existing.value}'),
        label = TextEditingController(text: existing?.label ?? def.defaultLabel);

  final BenefitKey def;
  bool included;
  bool flag;
  final TextEditingController count;
  final TextEditingController label;

  Object? get value => def.kind == BenefitKind.flag ? flag : int.tryParse(count.text.trim());
}

String _benefitsText(List<PlanBenefit> bs) => bs.map((b) => '${b.key} = ${b.value}  "${b.label}"').join('\n');

Future<bool> _editBenefits(BuildContext context, WidgetRef ref, Plan p) async {
  final api = ref.read(plansApiProvider);
  final existing = {for (final b in p.benefits) b.key: b};
  final rows = [for (final k in benefitKeys) _BenefitRow(k, existing[k.key])];
  final unknown = p.benefits.where((b) => !benefitKeys.any((k) => k.key == b.key)).toList();

  List<PlanBenefit> build() => [
        for (var i = 0; i < rows.length; i++)
          if (rows[i].included) PlanBenefit(key: rows[i].def.key, value: rows[i].value, label: rows[i].label.text.trim(), sortOrder: i),
      ];

  return showEditorDialog(
    context,
    title: 'Benefits of ${p.name}',
    width: 760,
    builder: (_, setState) => formColumn([
      const Text('Keys come from a fixed list the app and server understand. The label is what users see.'),
      if (unknown.isNotEmpty)
        Text('This plan has keys outside the list (${unknown.map((b) => b.key).join(', ')}). Saving removes them.',
            style: TextStyle(color: Theme.of(context).colorScheme.error)),
      for (final r in rows)
        Row(children: [
          SizedBox(
            width: 260,
            child: CheckboxListTile(
              dense: true,
              contentPadding: EdgeInsets.zero,
              controlAffinity: ListTileControlAffinity.leading,
              title: Text(r.def.key),
              value: r.included,
              onChanged: (v) => setState(() => r.included = v ?? false),
            ),
          ),
          SizedBox(
            width: 140,
            child: r.def.kind == BenefitKind.flag
                ? SwitchListTile(
                    dense: true,
                    title: const Text('On'),
                    value: r.flag,
                    onChanged: r.included ? (v) => setState(() => r.flag = v) : null,
                  )
                : FormText(label: 'Count', controller: r.count, enabled: r.included),
          ),
          const SizedBox(width: 12),
          Expanded(child: FormText(label: 'Label', controller: r.label, enabled: r.included)),
        ]),
    ]),
    validate: () {
      final e = <String, String>{};
      for (final r in rows.where((r) => r.included)) {
        if (r.label.text.trim().isEmpty) e[r.def.key] = 'label is required';
        if (r.def.kind == BenefitKind.count) {
          final n = int.tryParse(r.count.text.trim());
          if (n == null || n < 0) e[r.def.key] = 'count must be a whole number, 0 or more';
        }
      }
      return e;
    },
    confirmMessage: 'Replace the benefit list of ${p.name}? Active subscribers get the new benefits straight away.',
    confirmDetails: () => DiffView(before: _benefitsText(p.benefits), after: _benefitsText(build())),
    save: () => api.setBenefits(p.id!, build()),
  );
}

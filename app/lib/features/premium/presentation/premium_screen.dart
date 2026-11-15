import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:in_app_purchase/in_app_purchase.dart';

import '../../../core/env.dart';
import '../../../core/format/dates.dart';
import '../../../core/money/money.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/section.dart';
import '../application/premium_controller.dart';
import '../data/premium_models.dart';

/// `/premium`: current subscription and the plans for sale. Prices come from
/// the store listing when available, else from the plan (`pricePaise`);
/// nothing is hard-coded. Cancelling is done in the store.
class PremiumScreen extends ConsumerWidget {
  const PremiumScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final plans = ref.watch(premiumPlansProvider);
    final mine = ref.watch(mySubscriptionProvider);

    ref.listen<PurchaseState>(premiumPurchaseProvider, (prev, next) {
      final msg = next.message;
      if (msg != null && prev?.phase != next.phase) showInfoSnack(context, msg);
    });
    final purchase = ref.watch(premiumPurchaseProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Premium'),
        actions: [
          TextButton(
            onPressed: purchase.busy ? null : () => ref.read(premiumPurchaseProvider.notifier).restore(),
            child: const Text('Restore'),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          ref.invalidate(premiumPlansProvider);
          ref.invalidate(mySubscriptionProvider);
          await ref.read(premiumPlansProvider.future);
        },
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          children: [
            if (purchase.busy) const LinearProgressIndicator(),
            mine.when(
              skipLoadingOnRefresh: true,
              loading: () => const SizedBox.shrink(),
              error: (_, __) => const SizedBox.shrink(),
              data: (m) => m.active == null ? const SizedBox.shrink() : _CurrentPlan(sub: m.active!),
            ),
            AsyncValueView<List<Plan>>(
              value: plans,
              onRetry: () => ref.invalidate(premiumPlansProvider),
              data: (list) => list.isEmpty
                  ? const SizedBox(
                      height: 240,
                      child: EmptyView(
                        text: 'Premium is not available right now.',
                        icon: Icons.workspace_premium_outlined,
                      ),
                    )
                  : _PlanList(plans: list, active: mine.valueOrNull?.active, busy: purchase.busy),
            ),
            const Padding(
              padding: EdgeInsets.all(16),
              child: Text(
                'Subscriptions renew automatically until cancelled. Manage or cancel them in '
                'your Google Play or App Store account settings.',
                textAlign: TextAlign.center,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _CurrentPlan extends StatelessWidget {
  const _CurrentPlan({required this.sub});

  final Subscription sub;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Card(
      margin: const EdgeInsets.all(12),
      color: scheme.primaryContainer,
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(Icons.workspace_premium),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(sub.plan.name, style: Theme.of(context).textTheme.titleMedium),
                ),
                Chip(label: Text(humanize(sub.status))),
              ],
            ),
            if (sub.expiresAt != null)
              Text(sub.autoRenew
                  ? 'Renews on ${formatDate(sub.expiresAt)}'
                  : 'Ends on ${formatDate(sub.expiresAt)}'),
            const SizedBox(height: 8),
            for (final b in sub.plan.benefits)
              Row(children: [
                const Icon(Icons.check, size: 16),
                const SizedBox(width: 6),
                Expanded(child: Text(b.label)),
              ]),
          ],
        ),
      ),
    );
  }
}

class _PlanList extends ConsumerWidget {
  const _PlanList({required this.plans, required this.active, required this.busy});

  final List<Plan> plans;
  final Subscription? active;
  final bool busy;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final ids = plans.map(storeProductIdFor).whereType<String>().toList()..sort();
    final products = ref.watch(storeProductsProvider(ids.join(','))).valueOrNull ?? const {};
    final role = ref.watch(meProvider)?.primaryRole;

    // My audience first, then the rest.
    final audiences = {
      if (role != null && plans.any((p) => p.audience == role)) role,
      ...plans.map((p) => p.audience),
    }.toList();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final audience in audiences) ...[
          SectionHeader('For ${humanize(audience)}s'),
          for (final plan in plans.where((p) => p.audience == audience))
            _PlanCard(
              plan: plan,
              product: products[storeProductIdFor(plan)],
              isCurrent: active != null && active!.isActive && active!.plan.id == plan.id,
              busy: busy,
            ),
        ],
      ],
    );
  }
}

class _PlanCard extends ConsumerWidget {
  const _PlanCard({
    required this.plan,
    required this.product,
    required this.isCurrent,
    required this.busy,
  });

  final Plan plan;
  final ProductDetails? product;
  final bool isCurrent;
  final bool busy;

  /// Store price (localised, includes tax) when known, else the plan price.
  String get _price {
    final store = product?.price;
    final text = store != null && store.isNotEmpty ? store : formatPaise(plan.pricePaise, compact: true);
    return '$text / ${_period(plan.billingPeriod)}';
  }

  static String _period(String p) => switch (p) {
        'monthly' => 'month',
        'quarterly' => 'quarter',
        'yearly' => 'year',
        _ => humanize(p),
      };

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final ctrl = ref.read(premiumPurchaseProvider.notifier);
    final description = plan.description;
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(plan.name, style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 4),
            Text(_price, style: Theme.of(context).textTheme.titleLarge),
            if (description != null && description.isNotEmpty) ...[
              const SizedBox(height: 8),
              Text(description),
            ],
            const SizedBox(height: 8),
            for (final b in plan.benefits)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 2),
                child: Row(children: [
                  const Icon(Icons.check_circle_outline, size: 18),
                  const SizedBox(width: 8),
                  Expanded(child: Text(b.label)),
                ]),
              ),
            const SizedBox(height: 12),
            if (isCurrent)
              const Text('Your current plan')
            else
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  FilledButton(
                    onPressed: busy || product == null ? null : () => ctrl.buy(plan, product),
                    child: const Text('Subscribe'),
                  ),
                  if (Env.showMockFlows)
                    BusyButton(
                      label: 'Activate (test)',
                      style: BusyButtonStyle.outlined,
                      onPressed: busy ? null : () => ctrl.mockActivate(plan),
                    ),
                ],
              ),
            if (!isCurrent && product == null)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Text(
                  'Not available in the store on this device.',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ),
          ],
        ),
      ),
    );
  }
}

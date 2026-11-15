import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/format/format.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../application/dashboard_providers.dart';
import '../data/dashboard_api.dart';

class DashboardScreen extends ConsumerWidget {
  const DashboardScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(dashboardProvider);
    return PageScaffold(
      title: 'Dashboard',
      actions: [
        IconButton(tooltip: 'Refresh', icon: const Icon(Icons.refresh), onPressed: () => ref.invalidate(dashboardProvider)),
      ],
      body: AsyncBody<Dashboard>(
        value: value,
        onRetry: () => ref.invalidate(dashboardProvider),
        data: (d) => SingleChildScrollView(
          child: Wrap(spacing: 16, runSpacing: 16, children: [
            _Tile('Users', d.value('users', 'total'), [
              ('Verified', '${d.value('users', 'verified')}'),
              ('Minors', '${d.value('users', 'minors')}'),
              ('New, last 7 days', '${d.value('users', 'newLast7Days')}'),
            ]),
            _Tile('Events published', d.value('events', 'published'), [
              ('Pending review', '${d.value('events', 'pendingReview')}'),
              ('Upcoming', '${d.value('events', 'upcoming')}'),
            ]),
            _Tile('Organisers pending review', d.value('organisers', 'pendingReview'), const []),
            _Tile('Open reports', d.value('reports', 'open'), const []),
            _Tile('Active communities', d.value('communities', 'active'), const []),
            _Tile('Active subscriptions', d.value('revenue', 'activeSubscriptions'), [
              ('Listing fees paid, 30 days', formatPaise(d.value('revenue', 'listingPaidPaiseLast30Days'))),
              ('Listing fees refunded, 30 days', formatPaise(d.value('revenue', 'listingRefundedPaiseLast30Days'))),
            ]),
          ]),
        ),
      ),
    );
  }
}

class _Tile extends StatelessWidget {
  const _Tile(this.title, this.value, this.rows);

  final String title;
  final int value;
  final List<(String, String)> rows;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 280,
      child: Card(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(title, style: Theme.of(context).textTheme.bodyMedium),
            const SizedBox(height: 4),
            Text('$value', style: Theme.of(context).textTheme.headlineMedium),
            for (final r in rows)
              Padding(
                padding: const EdgeInsets.only(top: 4),
                child: Row(children: [Expanded(child: Text(r.$1)), Text(r.$2)]),
              ),
          ]),
        ),
      ),
    );
  }
}

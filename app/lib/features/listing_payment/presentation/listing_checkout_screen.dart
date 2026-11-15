import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/models/policy.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../events/application/event_providers.dart';
import '../../events/data/event_models.dart';
import '../application/listing_checkout_controller.dart';
import '../data/listing_models.dart';
import 'quote_breakdown.dart';

/// Pay the listing fee and submit an event.
class ListingCheckoutScreen extends ConsumerStatefulWidget {
  const ListingCheckoutScreen({super.key, required this.eventId});

  final int eventId;

  @override
  ConsumerState<ListingCheckoutScreen> createState() => _ListingCheckoutScreenState();
}

class _ListingCheckoutScreenState extends ConsumerState<ListingCheckoutScreen> {
  bool _agreed = false;

  Future<void> _pay(Event event) async {
    try {
      final outcome = await ref.read(listingCheckoutProvider).pay(event.id, event.summary.title);
      if (!mounted) return;
      switch (outcome) {
        case CheckoutOutcome.paid:
          final review = ref.read(appConfigProvider).eventRequireAdminApproval ?? false;
          showInfoSnack(context, review ? 'Paid. Your event is waiting for review.' : 'Paid. Your event is live.');
          context.pop();
        case CheckoutOutcome.pending:
          showInfoSnack(context, 'Payment is being confirmed. Check the event again in a minute.');
          context.pop();
        case CheckoutOutcome.cancelled:
          showInfoSnack(context, 'Payment cancelled.');
      }
    } on CheckoutError catch (e) {
      if (mounted) showInfoSnack(context, e.message);
    }
  }

  @override
  Widget build(BuildContext context) {
    final event = ref.watch(eventProvider(widget.eventId));
    final quote = ref.watch(listingQuoteProvider(widget.eventId));
    final refundPolicy = ref.watch(bootstrapProvider).valueOrNull?.policy(PolicyDocType.listingRefund);
    return Scaffold(
      appBar: AppBar(title: const Text('Listing fee')),
      body: AsyncValueView<Event>(
        value: event,
        onRetry: () => ref.invalidate(eventProvider(widget.eventId)),
        data: (e) => AsyncValueView<ListingQuote>(
          value: quote,
          onRetry: () => ref.invalidate(listingQuoteProvider(widget.eventId)),
          data: (q) => ListView(
            padding: const EdgeInsets.all(16),
            children: [
              Text(e.summary.title, style: Theme.of(context).textTheme.titleLarge),
              const SizedBox(height: 16),
              QuoteBreakdown(quote: q),
              const SizedBox(height: 16),
              if (refundPolicy != null)
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  title: Text(refundPolicy.title),
                  trailing: const Icon(Icons.chevron_right),
                  onTap: () => context.push(Routes.policy(refundPolicy.docType)),
                ),
              CheckboxListTile(
                contentPadding: EdgeInsets.zero,
                controlAffinity: ListTileControlAffinity.leading,
                value: _agreed,
                onChanged: (v) => setState(() => _agreed = v ?? false),
                title: const Text('I have read the listing refund policy'),
              ),
              const SizedBox(height: 16),
              BusyButton(
                label: q.totalPaise == 0 ? 'Submit event' : 'Pay and submit',
                onPressed: _agreed ? () => _pay(e) : null,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

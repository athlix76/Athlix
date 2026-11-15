import 'package:flutter/material.dart';

import '../../../core/money/money.dart';
import '../data/listing_models.dart';

/// Shows how a listing fee was worked out. Every number comes from the quote.
class QuoteBreakdown extends StatelessWidget {
  const QuoteBreakdown({super.key, required this.quote});

  final ListingQuote quote;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    Widget row(String label, String value, {bool bold = false}) => Padding(
          padding: const EdgeInsets.symmetric(vertical: 2),
          child: Row(
            children: [
              Expanded(child: Text(label)),
              Text(value, style: bold ? theme.textTheme.titleMedium : null),
            ],
          ),
        );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (quote.isFreeEvent)
          row('Free event listing', formatPaise(quote.baseFeePaise))
        else ...[
          row('Entry fee × slots', formatPaise(quote.grossEntryPaise)),
          row('Listing rate', formatBp(quote.percentBp)),
          row('Minimum / maximum', '${formatPaise(quote.minFeePaise)} / ${formatPaise(quote.maxFeePaise)}'),
          row('Listing fee', formatPaise(quote.baseFeePaise)),
        ],
        row('GST (${formatBp(quote.gstBp)})', formatPaise(quote.gstPaise)),
        const Divider(),
        row('Total', formatPaise(quote.totalPaise), bold: true),
        const SizedBox(height: 8),
        Text(
          'Refund if cancelled: ${_pct(quote.refundPolicy.beforeFirstRegistrationPercent)} before the '
          'first registration, ${_pct(quote.refundPolicy.afterRegistrationsPercent)} after, '
          '${_pct(quote.refundPolicy.onAdminRejectionPercent)} if rejected in review.',
          style: theme.textTheme.bodySmall,
        ),
        const SizedBox(height: 4),
        Text(
          'This is the fee for listing the event on Athlix. Entry fees are paid by players to '
          'you directly; Athlix never collects them.',
          style: theme.textTheme.bodySmall,
        ),
      ],
    );
  }

  static String _pct(int? v) => v == null ? '-' : '$v%';
}

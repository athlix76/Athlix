import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/api/api_error.dart';
import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/models/listing.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/cursor_list.dart';
import '../../../core/widgets/date_time_field.dart';
import '../../../core/widgets/permission_gate.dart';
import '../application/payments_providers.dart';
import '../data/listing_orders_api.dart';

/// Listing fee orders (the only money Athlix collects for events) and refunds.
class ListingOrdersScreen extends ConsumerStatefulWidget {
  const ListingOrdersScreen({super.key});

  @override
  ConsumerState<ListingOrdersScreen> createState() => _ListingOrdersScreenState();
}

class _ListingOrdersScreenState extends ConsumerState<ListingOrdersScreen> {
  final _listKey = GlobalKey<CursorListState<ListingOrder>>();
  final _eventId = TextEditingController();
  String? _status;
  DateTime? _from;
  DateTime? _to;

  @override
  void dispose() {
    _eventId.dispose();
    super.dispose();
  }

  void _apply() => _listKey.currentState?.reload();

  ListingOrderFilters get _filters =>
      ListingOrderFilters(status: _status, eventId: int.tryParse(_eventId.text.trim()), from: _from, to: _to);

  Future<void> _refund(ListingOrder o) async {
    final api = ref.read(listingOrdersApiProvider);
    final amount = TextEditingController(text: paiseToInput(o.refundablePaise));
    final r = await confirmAction<ListingOrder>(
      context,
      title: 'Refund listing order #${o.orderId}?',
      message: 'Paid ${formatPaise(o.totalPaise)}, already refunded ${formatPaise(o.refundPaise)}. '
          'At most ${formatPaise(o.refundablePaise)} can still be refunded. The refund goes back through ${o.provider}.',
      confirmLabel: 'Refund',
      destructive: true,
      details: TextField(
        controller: amount,
        keyboardType: const TextInputType.numberWithOptions(decimal: true),
        decoration: const InputDecoration(labelText: 'Amount (₹) *', border: OutlineInputBorder(), prefixText: '₹ '),
      ),
      reason: const ReasonSpec(hint: 'Kept in the audit log'),
      run: (reason) {
        final paise = parseRupeesToPaise(amount.text);
        if (paise == null || paise <= 0) {
          throw ApiError.invalidInput({'amount': 'enter an amount in rupees, up to 2 decimals'});
        }
        if (paise > o.refundablePaise) {
          throw ApiError.invalidInput({'amount': 'more than the refundable ${formatPaise(o.refundablePaise)}'});
        }
        return api.refund(o.orderId, paise, reason!);
      },
      successMessage: 'Refund requested',
    );
    if (r != null) _apply();
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(listingOrdersApiProvider);
    return PageScaffold(
      title: 'Listing payments',
      subtitle: 'Listing fees organisers paid to publish events. Entry fees are paid to organisers directly and are not here.',
      filters: Wrap(spacing: 12, runSpacing: 12, crossAxisAlignment: WrapCrossAlignment.center, children: [
        FilterDropdown(label: 'Status', value: _status, options: listingOrderStatuses, onChanged: (v) {
          setState(() => _status = v);
          _apply();
        }),
        FilterTextField(label: 'Event id', controller: _eventId, onSubmitted: (_) => _apply(), width: 160),
        SizedBox(
          width: 200,
          child: DateTimeField(label: 'From', value: _from, dateOnly: true, allowClear: true, onChanged: (v) {
            setState(() => _from = v);
            _apply();
          }),
        ),
        SizedBox(
          width: 200,
          child: DateTimeField(label: 'To', value: _to, dateOnly: true, allowClear: true, onChanged: (v) {
            setState(() => _to = v);
            _apply();
          }),
        ),
      ]),
      body: CursorList<ListingOrder>(
        key: _listKey,
        load: (cursor) => api.list(_filters, cursor),
        emptyText: 'No listing orders match.',
        itemBuilder: (context, o) {
          final q = o.quote;
          return ListTile(
            title: Text('Order #${o.orderId}  ·  ${formatPaise(o.totalPaise)}'
                '${o.refundPaise > 0 ? '  ·  refunded ${formatPaise(o.refundPaise)}' : ''}'),
            subtitle: Text([
              'event #${o.eventId} (${o.eventStatus ?? '-'})',
              if (o.organiser != null) o.organiser!.label,
              if (q != null) 'fee ${formatPaise(q.baseFeePaise)} + GST ${formatPaise(q.gstPaise)} (${q.ruleName})',
              '${o.provider}${o.providerOrderId != null ? ' ${o.providerOrderId}' : ''}',
              'paid ${formatDateTime(o.paidAt)}',
            ].join('  ·  ')),
            onTap: () => context.go('/events/${o.eventId}'),
            trailing: Row(mainAxisSize: MainAxisSize.min, children: [
              StatusChip(o.status),
              if ((o.status == 'paid' || o.status == 'partially_refunded') && o.refundablePaise > 0)
                IfCan(
                  Perm.paymentsRefund,
                  child: Padding(
                    padding: const EdgeInsets.only(left: 8),
                    child: OutlinedButton(onPressed: () => _refund(o), child: const Text('Refund')),
                  ),
                ),
            ]),
          );
        },
      ),
    );
  }
}

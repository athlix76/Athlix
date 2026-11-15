import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_error.dart';
import '../../../core/format/format.dart';
import '../../../core/models/listing.dart';
import '../../../core/widgets/api_error_view.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../sports/data/sports_api.dart';
import '../application/fee_rules_providers.dart';
import '../data/fee_rules_api.dart';

/// Live fee calculator against the saved rules (`POST /admin/fee-rules/preview`).
class FeePreviewCard extends ConsumerStatefulWidget {
  const FeePreviewCard({super.key, required this.sports});

  final List<AdminSport> sports;

  @override
  ConsumerState<FeePreviewCard> createState() => _FeePreviewCardState();
}

class _FeePreviewCardState extends ConsumerState<FeePreviewCard> {
  final _fee = TextEditingController();
  final _slots = TextEditingController();
  int? _sportId;
  String? _eventType;
  bool _busy = false;
  Object? _error;
  ListingQuote? _quote;

  @override
  void dispose() {
    _fee.dispose();
    _slots.dispose();
    super.dispose();
  }

  Future<void> _calculate() async {
    final fee = parseRupeesToPaise(_fee.text);
    final slots = int.tryParse(_slots.text.trim());
    if (fee == null || slots == null || slots <= 0) {
      setState(() => _error = ApiError.invalidInput({
            if (fee == null) 'entry fee': 'an amount in rupees (0 for a free event)',
            if (slots == null || slots <= 0) 'max slots': 'a whole number above 0',
          }));
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final q = await ref.read(feeRulesApiProvider).preview(sportId: _sportId, eventType: _eventType, entryFeePaise: fee, maxSlots: slots);
      if (mounted) setState(() => _quote = q);
    } catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final q = _quote;
    return SectionCard(
      title: 'Fee preview (saved rules)',
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Wrap(spacing: 12, runSpacing: 12, crossAxisAlignment: WrapCrossAlignment.center, children: [
          SizedBox(
            width: 200,
            child: FormDropdown<int>(
              label: 'Sport',
              value: _sportId,
              nullLabel: 'Any sport',
              items: [for (final s in widget.sports) (s.id, s.name)],
              onChanged: (v) => setState(() => _sportId = v),
            ),
          ),
          SizedBox(
            width: 180,
            child: FormDropdown<String>(
              label: 'Event type',
              value: _eventType,
              nullLabel: 'Any type',
              items: [for (final t in eventTypes) (t, t)],
              onChanged: (v) => setState(() => _eventType = v),
            ),
          ),
          SizedBox(width: 180, child: FormText(label: 'Entry fee per slot', controller: _fee, prefixText: '₹ ')),
          SizedBox(width: 140, child: FormText(label: 'Max slots', controller: _slots)),
          FilledButton(
            onPressed: _busy ? null : _calculate,
            child: _busy ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)) : const Text('Calculate'),
          ),
        ]),
        if (_error != null) ...[const SizedBox(height: 12), ApiErrorBanner(error: _error!)],
        if (q != null) ...[
          const SizedBox(height: 12),
          KeyValueList([
            ('Rule used', textValue('${q.ruleName} (#${q.feeRuleId})')),
            ('Gross entry', textValue(q.isFreeEvent ? 'free event' : formatPaise(q.grossEntryPaise))),
            ('Percent', textValue(formatBp(q.percentBp))),
            ('Min / max', textValue('${formatPaise(q.minFeePaise)} / ${q.maxFeePaise == null ? 'no cap' : formatPaise(q.maxFeePaise)}')),
            ('Base fee', textValue(formatPaise(q.baseFeePaise))),
            ('GST (${formatBp(q.gstBp)})', textValue(formatPaise(q.gstPaise))),
            ('Total the organiser pays', textValue(formatPaise(q.totalPaise))),
            ('Refund policy', textValue(q.refundPolicy.entries.map((e) => '${e.key}: ${e.value}%').join(', '))),
          ]),
        ],
      ]),
    );
  }
}

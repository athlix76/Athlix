import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../application/location_identity_controller.dart';
import '../data/location_identity_models.dart';

/// Settings → Location identity: one card each for native and residence.
class LocationIdentityScreen extends ConsumerWidget {
  const LocationIdentityScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(locationIdentitiesProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Location identity')),
      body: AsyncValueView<LocationIdentities>(
        value: value,
        onRetry: () => ref.invalidate(locationIdentitiesProvider),
        data: (ids) => RefreshIndicator(
          onRefresh: () => ref.refresh(locationIdentitiesProvider.future),
          child: ListView(
            padding: const EdgeInsets.all(16),
            children: [
              const Text(
                'Some events are only for players from a district or state. '
                'Your native place and where you live decide which ones you can join. '
                'Organisers only see "Native" or "Residence" and the district, never your documents.',
              ),
              const SizedBox(height: 16),
              _SlotCard(type: IdentityType.native, title: 'Native place', slot: ids.native),
              const SizedBox(height: 12),
              _SlotCard(type: IdentityType.residence, title: 'Residence', slot: ids.residence),
            ],
          ),
        ),
      ),
    );
  }
}

class _SlotCard extends ConsumerWidget {
  const _SlotCard({required this.type, required this.title, required this.slot});

  final String type;
  final String title;
  final IdentitySlot slot;

  Future<void> _withdraw(BuildContext context, WidgetRef ref, LocationIdentity id) async {
    final isPending = id.status == 'pending';
    final ok = await confirm(
      context,
      title: isPending ? 'Withdraw submission?' : 'Remove this identity?',
      body: isPending
          ? 'The proof you uploaded is deleted straight away.'
          : 'Events limited to this place will no longer accept you until you verify again.',
      ok: isPending ? 'Withdraw' : 'Remove',
    );
    if (!ok || !context.mounted) return;
    try {
      await ref.read(locationIdentityControllerProvider).withdraw(id.id);
      if (context.mounted) showInfoSnack(context, isPending ? 'Withdrawn.' : 'Removed.');
    } catch (e) {
      if (context.mounted) showErrorSnack(context, e);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final theme = Theme.of(context);
    final current = slot.current;
    final pending = slot.pending;
    final shown = slot.displayStatus == DisplayStatus.pending ? (pending ?? current) : (current ?? pending);
    final next = slot.nextChangeAt;
    final cooling = next != null && next.isAfter(DateTime.now());

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(child: Text(title, style: theme.textTheme.titleMedium)),
                _StatusChip(status: slot.displayStatus),
              ],
            ),
            if (shown != null && shown.place.isNotEmpty) ...[
              const SizedBox(height: 8),
              Row(children: [
                const Icon(Icons.place_outlined, size: 18),
                const SizedBox(width: 6),
                Expanded(child: Text(shown.place)),
              ]),
            ],
            if (current != null && current.fromDigilocker && slot.displayStatus == DisplayStatus.verified)
              const Padding(
                padding: EdgeInsets.only(top: 4),
                child: Text('From DigiLocker'),
              ),
            if (slot.displayStatus == DisplayStatus.verified && slot.expiresInDays != null)
              Padding(
                padding: const EdgeInsets.only(top: 4),
                child: Text('Expires in ${slot.expiresInDays} days'),
              ),
            if (slot.displayStatus == DisplayStatus.rejected && current?.rejectionReason != null)
              Padding(
                padding: const EdgeInsets.only(top: 4),
                child: Text(
                  'Rejected: ${current!.rejectionReason}',
                  style: TextStyle(color: theme.colorScheme.error),
                ),
              ),
            if (slot.displayStatus == DisplayStatus.expired)
              const Padding(
                padding: EdgeInsets.only(top: 4),
                child: Text('Expired. Upload a new proof to stay eligible.'),
              ),
            if (pending != null && slot.displayStatus != DisplayStatus.pending)
              Padding(
                padding: const EdgeInsets.only(top: 4),
                child: Text('A new submission (${pending.place}) is in review.'),
              ),
            if (cooling)
              Padding(
                padding: const EdgeInsets.only(top: 4),
                child: Text('You can move to a different district from ${formatDate(next)}.'),
              ),
            const SizedBox(height: 12),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                if (slot.canSubmit && pending == null)
                  FilledButton.tonal(
                    onPressed: () => context.push(Routes.locationIdentitySubmit(type)),
                    child: Text(_submitLabel(slot)),
                  ),
                if (pending != null)
                  OutlinedButton(
                    onPressed: () => _withdraw(context, ref, pending),
                    child: const Text('Withdraw'),
                  ),
                if (current != null && current.status == 'verified' && current.canWithdraw)
                  TextButton(
                    onPressed: () => _withdraw(context, ref, current),
                    child: const Text('Remove'),
                  ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  static String _submitLabel(IdentitySlot slot) => switch (slot.displayStatus) {
        DisplayStatus.verified => 'Update',
        DisplayStatus.expired => 'Renew',
        DisplayStatus.rejected => 'Try again',
        _ => 'Add proof',
      };
}

class _StatusChip extends StatelessWidget {
  const _StatusChip({required this.status});

  final String status;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final (label, color) = switch (status) {
      DisplayStatus.verified => ('Verified', scheme.primaryContainer),
      DisplayStatus.pending => ('In review', scheme.tertiaryContainer),
      DisplayStatus.rejected => ('Rejected', scheme.errorContainer),
      DisplayStatus.expired => ('Expired', scheme.errorContainer),
      _ => ('Not added', scheme.surfaceContainerHighest),
    };
    return Chip(
      label: Text(label),
      backgroundColor: color,
      side: BorderSide.none,
      visualDensity: VisualDensity.compact,
    );
  }
}

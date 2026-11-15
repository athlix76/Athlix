import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/json.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/cursor_list.dart';
import '../application/events_providers.dart';
import '../data/events_api.dart';

class EventDetailScreen extends ConsumerWidget {
  const EventDetailScreen({super.key, required this.eventId});

  final int eventId;

  Future<void> _run(
    BuildContext context,
    WidgetRef ref, {
    required String title,
    required String message,
    required String label,
    required Future<AdminEvent> Function(String? reason) run,
    ReasonSpec? reason,
    bool destructive = false,
  }) async {
    final r = await confirmAction<AdminEvent>(
      context,
      title: title,
      message: message,
      confirmLabel: label,
      destructive: destructive,
      reason: reason,
      run: run,
      successMessage: 'Done',
    );
    if (r != null) ref.invalidate(eventDetailProvider(eventId));
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final api = ref.read(eventsApiProvider);
    final value = ref.watch(eventDetailProvider(eventId));
    return PageScaffold(
      title: 'Event #$eventId',
      actions: [
        IconButton(tooltip: 'Reload', icon: const Icon(Icons.refresh), onPressed: () => ref.invalidate(eventDetailProvider(eventId))),
      ],
      body: AsyncBody<AdminEvent>(
        value: value,
        onRetry: () => ref.invalidate(eventDetailProvider(eventId)),
        data: (e) {
          final r = e.raw;
          return SingleChildScrollView(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              SectionCard(
                title: e.title,
                actions: [
                  StatusChip(e.status),
                  if (e.status == 'pending_review') ...[
                    const SizedBox(width: 8),
                    OutlinedButton(
                      onPressed: () => _run(context, ref,
                          title: 'Reject event?',
                          message: 'The organiser sees the reason. The listing fee is refunded by the configured rejection percentage.',
                          label: 'Reject',
                          destructive: true,
                          reason: const ReasonSpec(hint: 'Shown to the organiser'),
                          run: (reason) => api.reject(eventId, reason!)),
                      child: const Text('Reject'),
                    ),
                    const SizedBox(width: 8),
                    FilledButton(
                      onPressed: () => _run(context, ref,
                          title: 'Approve and publish?',
                          message: 'The event becomes visible to players.',
                          label: 'Approve',
                          run: (_) => api.approve(eventId)),
                      child: const Text('Approve'),
                    ),
                  ],
                  if (e.status == 'published') ...[
                    const SizedBox(width: 8),
                    OutlinedButton(
                      onPressed: () => _run(context, ref,
                          title: 'Remove event?',
                          message: 'The event is taken down and registered players are told it is cancelled.',
                          label: 'Remove',
                          destructive: true,
                          reason: const ReasonSpec(hint: 'Shown to the organiser'),
                          run: (reason) => api.remove(eventId, reason!)),
                      child: const Text('Remove'),
                    ),
                    const SizedBox(width: 8),
                    OutlinedButton(
                      onPressed: () => _run(context, ref,
                          title: e.isFeatured ? 'Stop featuring?' : 'Feature this event?',
                          message: e.isFeatured
                              ? 'The event goes back to normal ordering in discovery.'
                              : 'The event is pinned near the top of discovery.',
                          label: e.isFeatured ? 'Unfeature' : 'Feature',
                          run: (_) => api.setFeatured(eventId, !e.isFeatured)),
                      child: Text(e.isFeatured ? 'Unfeature' : 'Feature'),
                    ),
                  ],
                  const SizedBox(width: 8),
                  OutlinedButton(
                    onPressed: () => showRegistrationsDialog(context, api, eventId),
                    child: Text('Registrations (${e.registrationsCount})'),
                  ),
                ],
                child: KeyValueList([
                  ('Organiser', textValue(e.organiser?.label)),
                  ('Sport', textValue(e.sport?.name)),
                  ('Type', textValue('${e.eventType} · ${r['participantType'] ?? '-'}')),
                  ('Level / gender', textValue('${r['competitionLevel'] ?? '-'} · ${r['genderCategory'] ?? '-'}')),
                  ('Age', textValue('${r['ageMin'] ?? 'any'} to ${r['ageMax'] ?? 'any'}')),
                  ('Venue', textValue([e.venueName, r['addressLine'], e.city, r['district'], e.state, r['pincode']].whereType<Object>().join(', '))),
                  ('Starts', textValue(formatDateTime(e.startsAt))),
                  ('Ends', textValue(formatDateTime(asDate(r['endsAt'])))),
                  ('Registration deadline', textValue(formatDateTime(e.registrationDeadline))),
                  ('Entry fee (paid to organiser)', textValue(formatPaise(e.entryFeePaise))),
                  ('Slots', textValue('${e.registrationsCount} of ${e.maxSlots} taken')),
                  ('Featured', textValue(e.isFeatured ? 'yes' : 'no')),
                  ('Contact', textValue('${r['contactName'] ?? '-'} · ${r['contactPhoneMasked'] ?? '-'}')),
                  ('Rejection reason', textValue(r['rejectionReason'])),
                  ('Cancel reason', textValue(r['cancelReason'])),
                  ('Published', textValue(formatDateTime(asDate(r['publishedAt'])))),
                ]),
              ),
              if (e.bannerUrl != null)
                SectionCard(
                  title: 'Banner',
                  child: Align(
                    alignment: Alignment.centerLeft,
                    child: ConstrainedBox(
                      constraints: const BoxConstraints(maxHeight: 240),
                      child: Image.network(e.bannerUrl!, errorBuilder: (_, __, ___) => const Text('Banner could not load (link may have expired).')),
                    ),
                  ),
                ),
              SectionCard(title: 'Category and eligibility', child: _EligibilitySummary(r)),
              SectionCard(
                title: 'Details',
                child: KeyValueList([
                  ('Description', textValue(r['description'])),
                  ('Rules', textValue(r['rules'])),
                  ('Prizes', textValue(r['prizeInfo'])),
                  ('Payment modes', textValue((r['paymentModes'] as List?)?.join(', '))),
                  ('Payment instructions', textValue(r['paymentInstructions'])),
                ]),
              ),
              SectionCard(
                title: 'Listing orders',
                child: e.listingOrders.isEmpty
                    ? const Text('No listing orders.')
                    : Column(children: [
                        for (final o in e.listingOrders)
                          ListTile(
                            dense: true,
                            title: Text('Order #${o.orderId}  ·  ${formatPaise(o.totalPaise)}'),
                            subtitle: Text('refunded ${formatPaise(o.refundPaise)}  ·  paid ${formatDateTime(o.paidAt)}  ·  ${o.provider}'),
                            trailing: StatusChip(o.status),
                          ),
                      ]),
              ),
            ]),
          );
        },
      ),
    );
  }
}


/// `category` + `eligibility` (EligibilityRule as stored, with names).
class _EligibilitySummary extends StatelessWidget {
  const _EligibilitySummary(this.r);

  final Json r;

  @override
  Widget build(BuildContext context) {
    final rule = asJsonOrNull(r['eligibility']);
    final type = asString(rule?['type'], 'open');
    final place = [asJsonOrNull(rule?['district'])?['name'], asJsonOrNull(rule?['state'])?['name']].whereType<Object>().join(', ');
    return KeyValueList([
      ('Category', textValue(r['category'])),
      ('Summary', textValue(rule?['summary'] ?? (type == 'open' ? 'Open to all' : null))),
      ('Rule', textValue(type == 'open' ? 'open' : '$type · ${rule?['scope'] ?? '-'}${place.isEmpty ? '' : ' · $place'}')),
      if (type != 'open') ('Verified identity required', textValue(asBool(rule?['requireVerified']) ? 'yes' : 'no (pending accepted)')),
    ]);
  }
}

/// Registrations of one event in a dialog (the detail page scrolls as a
/// whole, so the paged list gets its own box).
Future<void> showRegistrationsDialog(BuildContext context, EventsApi api, int eventId) => showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('Registrations · event #$eventId'),
        content: SizedBox(
          width: 640,
          height: 520,
          child: CursorList<AdminRegistration>(
            load: (cursor) => api.registrations(eventId, cursor: cursor),
            emptyText: 'No registrations yet.',
            itemBuilder: (context, reg) {
              final who = reg.user?.displayName ?? 'User';
              final team = reg.teamName == null ? '' : '  ·  team ${reg.teamName}';
              final flag = reg.eligibilityFlag == 'none' ? '' : '  ·  flag: ${reg.eligibilityFlag.replaceAll('_', ' ')}';
              return ListTile(
                dense: true,
                title: Text('$who (#${reg.user?.id ?? '-'})$team'),
                subtitle: Text('${reg.qualifiedLabel.isEmpty ? 'open category' : reg.qualifiedLabel}'
                    '  ·  payment ${reg.paymentStatus.replaceAll('_', ' ')}  ·  ${formatDateTime(reg.createdAt)}$flag'),
                trailing: StatusChip(reg.status),
              );
            },
          ),
        ),
        actions: [TextButton(onPressed: () => Navigator.of(ctx).pop(), child: const Text('Close'))],
      ),
    );

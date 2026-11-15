import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../core/format/dates.dart';
import '../../../core/models/policy.dart';
import '../../../core/money/money.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/section.dart';
import '../../../core/widgets/user_avatar.dart';
import '../../../core/widgets/user_tile.dart';
import '../../ads/presentation/ad_slot.dart';
import '../../chat/data/chat_api.dart';
import '../../location_identity/application/eligibility_view.dart';
import '../../location_identity/presentation/eligibility_chip.dart';
import '../../location_identity/presentation/not_eligible_sheet.dart';
import '../../registrations/application/registration_controller.dart';
import '../../reports/presentation/report_sheet.dart';
import '../application/event_providers.dart';
import '../data/event_models.dart';

class EventDetailScreen extends ConsumerWidget {
  const EventDetailScreen({super.key, required this.eventId});

  final int eventId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(eventProvider(eventId));
    return Scaffold(
      appBar: AppBar(
        title: const Text('Event'),
        actions: [
          if (value.valueOrNull case final e? when !e.isOwner)
            IconButton(
              tooltip: 'Report',
              icon: const Icon(Icons.flag_outlined),
              onPressed: () => showReportSheet(context, targetType: ReportTarget.event, targetId: e.id),
            ),
        ],
      ),
      body: AsyncValueView<Event>(
        value: value,
        onRetry: () => ref.invalidate(eventProvider(eventId)),
        data: (e) => RefreshIndicator(
          onRefresh: () => ref.refresh(eventProvider(eventId).future),
          child: _Body(event: e),
        ),
      ),
    );
  }
}

class _Body extends ConsumerWidget {
  const _Body({required this.event});

  final Event event;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = event.summary;
    final theme = Theme.of(context);
    final now = DateTime.now();
    final reg = event.myRegistration;
    return ListView(
      padding: const EdgeInsets.only(bottom: 32),
      children: [
        if (s.bannerUrl != null) SignedImage(url: s.bannerUrl!, height: 180),
        Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(s.title, style: theme.textTheme.headlineSmall),
              const SizedBox(height: 4),
              Text(
                [s.sport?.name, humanize(s.eventType), humanize(s.competitionLevel), humanize(s.genderCategory)]
                    .whereType<String>()
                    .join(' · '),
              ),
              if (!s.isPublished) ...[
                const SizedBox(height: 8),
                Chip(label: Text('Status: ${humanize(s.status)}')),
              ],
              if (event.rejectionReason != null)
                Text('Rejected: ${event.rejectionReason}', style: TextStyle(color: theme.colorScheme.error)),
              if (event.cancelReason != null) Text('Cancelled: ${event.cancelReason}'),
            ],
          ),
        ),
        InfoRow(label: 'Starts', value: formatDateTime(s.startsAt), icon: Icons.schedule),
        InfoRow(label: 'Ends', value: formatDateTime(s.endsAt), icon: Icons.schedule_outlined),
        InfoRow(label: 'Register by', value: formatDateTime(s.registrationDeadline), icon: Icons.timer_outlined),
        InfoRow(
          label: 'Venue',
          value: [s.venueName, event.addressLine, s.city, s.district, s.state, event.pincode]
              .whereType<String>()
              .where((v) => v.isNotEmpty)
              .join(', '),
          icon: Icons.place_outlined,
        ),
        InfoRow(label: 'Age', value: s.ageText, icon: Icons.cake_outlined),
        if (s.isTeam)
          InfoRow(
            label: 'Team size',
            value: '${event.teamSizeMin ?? '-'} to ${event.teamSizeMax ?? '-'}',
            icon: Icons.groups_outlined,
          ),
        InfoRow(label: 'Entry fee', value: formatEntryFee(s.entryFeePaise), icon: Icons.payments_outlined),
        InfoRow(
          label: 'Slots',
          value: '${s.slotsLeft} left of ${s.maxSlots} (${s.registrationsCount} registered)',
          icon: Icons.event_seat_outlined,
        ),
        _EligibilityInfo(event: s, isOwner: event.isOwner),
        const _EntryFeeDisclaimer(),
        if (event.isOwner) _OwnerActions(event: event) else _PlayerActions(event: event, now: now),
        if (reg != null && !event.isOwner)
          ListTile(
            leading: const Icon(Icons.confirmation_number_outlined),
            title: Text('Your registration: ${humanize(reg.status)}'),
            subtitle: Text('Payment: ${humanize(reg.paymentStatus)}'
                '${reg.paymentNote == null ? '' : ' (${reg.paymentNote})'}'),
          ),
        if (event.hasGroupChat && (event.isOwner || (reg?.isActive ?? false)))
          ListTile(
            leading: const Icon(Icons.forum_outlined),
            title: const Text('Event group chat'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () async {
              try {
                final c = await ref.read(chatApiProvider).eventGroup(event.id);
                if (context.mounted) context.push(Routes.conversation(c.id));
              } catch (e) {
                if (context.mounted) showErrorSnack(context, e);
              }
            },
          ),
        if (s.organiser != null) ...[
          const SectionHeader('Organiser'),
          UserTile(user: s.organiser!),
        ],
        _TextSection('About', event.description),
        _TextSection('Rules', event.rules),
        _TextSection('Prizes', event.prizeInfo),
        if (event.paymentModes.isNotEmpty)
          _TextSection('How to pay the organiser', [
            event.paymentModes.map(humanize).join(', '),
            if (event.paymentInstructions != null) event.paymentInstructions!,
          ].join('\n')),
        if (!event.isOwner) AdSlot(placement: AdPlacement.eventDetail, sportId: s.sport?.id, state: s.state),
      ],
    );
  }
}

class _TextSection extends StatelessWidget {
  const _TextSection(this.title, this.text);

  final String title;
  final String? text;

  @override
  Widget build(BuildContext context) {
    final t = text;
    if (t == null || t.trim().isEmpty) return const SizedBox.shrink();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SectionHeader(title),
        Padding(padding: const EdgeInsets.symmetric(horizontal: 16), child: SelectableText(t)),
      ],
    );
  }
}

/// Athlix never collects entry fees: shown on every event.
/// "Who can join" line from `eligibility.summary` plus the caller's standing.
class _EligibilityInfo extends StatelessWidget {
  const _EligibilityInfo({required this.event, required this.isOwner});

  final EventSummary event;
  final bool isOwner;

  @override
  Widget build(BuildContext context) {
    final rule = event.eligibility;
    if (rule == null || rule.isOpen) return const SizedBox.shrink();
    final view = EligibilityView.from(event.myEligibility);
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      child: Card(
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(children: [
                const Icon(Icons.place_outlined, size: 18),
                const SizedBox(width: 6),
                Text('Who can join', style: Theme.of(context).textTheme.titleSmall),
              ]),
              const SizedBox(height: 6),
              Text(rule.summary ?? 'Limited to players from a specific place.'),
              if (!isOwner) ...[
                const SizedBox(height: 6),
                EligibilityChip(view: view),
                if (!view.eligible && view.reasonText != null) Text(view.reasonText!),
                if (view.ctaLabel != null && view.ctaRoute != null)
                  Align(
                    alignment: Alignment.centerLeft,
                    child: TextButton(
                      onPressed: () => context.push(view.ctaRoute!),
                      child: Text(view.ctaLabel!),
                    ),
                  ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class _EntryFeeDisclaimer extends StatelessWidget {
  const _EntryFeeDisclaimer();

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.all(16),
      child: ListTile(
        leading: const Icon(Icons.info_outline),
        title: const Text('Athlix does not collect entry fees'),
        subtitle: const Text('Pay the organiser directly. Athlix only lists the event and records '
            'what the organiser marks as paid.'),
        onTap: () => context.push(Routes.policy(PolicyDocType.entryFeeDisclaimer)),
      ),
    );
  }
}

class _PlayerActions extends ConsumerWidget {
  const _PlayerActions({required this.event, required this.now});

  final Event event;
  final DateTime now;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final reg = event.myRegistration;
    final active = reg?.isActive ?? false;
    final open = event.summary.registrationOpen(now);
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: Wrap(
        spacing: 8,
        runSpacing: 8,
        children: [
          if (!active && open)
            FilledButton.icon(
              onPressed: () {
                final me = event.summary.myEligibility;
                if (me != null && !me.eligible) {
                  showNotEligibleSheet(context, NotEligibleInfo(reason: me.reason, action: me.action));
                  return;
                }
                context.push(Routes.eventRegister(event.id));
              },
              icon: const Icon(Icons.how_to_reg),
              label: Text(event.summary.slotsLeft > 0 ? 'Register' : 'Join waitlist'),
            ),
          if (active)
            BusyButton(
              label: 'Withdraw',
              style: BusyButtonStyle.outlined,
              onPressed: () async {
                final ok = await confirm(context,
                    title: 'Withdraw registration?', body: 'Your slot may go to the next player.', ok: 'Withdraw');
                if (!ok) return;
                await ref.read(registrationControllerProvider).withdraw(event.id);
              },
            ),
          BusyButton(
            label: 'Contact organiser',
            style: BusyButtonStyle.outlined,
            icon: Icons.call_outlined,
            onPressed: () async {
              try {
                final c = await ref.read(eventControllerProvider).contact(event.id);
                if (context.mounted) await _showContact(context, c);
              } catch (e) {
                if (!context.mounted) return;
                if (await showNotEligibleSheetIfNeeded(context, e)) return;
                rethrow;
              }
            },
          ),
        ],
      ),
    );
  }
}

Future<void> _showContact(BuildContext context, EventContact c) {
  final digits = c.contactPhone.replaceAll(RegExp(r'[^0-9]'), '');
  return showModalBottomSheet<void>(
    context: context,
    builder: (ctx) => SafeArea(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(c.contactName, style: Theme.of(ctx).textTheme.titleLarge),
            SelectableText(c.contactPhone),
            if (c.paymentModes.isNotEmpty) Text('Payment: ${c.paymentModes.map(humanize).join(', ')}'),
            if (c.paymentInstructions != null) Text(c.paymentInstructions!),
            const SizedBox(height: 12),
            Row(
              children: [
                FilledButton.icon(
                  onPressed: () => launchUrl(Uri(scheme: 'tel', path: c.contactPhone)),
                  icon: const Icon(Icons.call),
                  label: const Text('Call'),
                ),
                const SizedBox(width: 8),
                OutlinedButton.icon(
                  onPressed: () => launchUrl(
                    Uri.https('wa.me', '/$digits'),
                    mode: LaunchMode.externalApplication,
                  ),
                  icon: const Icon(Icons.chat_outlined),
                  label: const Text('WhatsApp'),
                ),
              ],
            ),
          ],
        ),
      ),
    ),
  );
}

class _OwnerActions extends ConsumerWidget {
  const _OwnerActions({required this.event});

  final Event event;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final status = event.status;
    final ctrl = ref.read(eventControllerProvider);
    final editable = !const ['pending_review', 'completed', 'cancelled', 'removed'].contains(status);
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: Wrap(
        spacing: 8,
        runSpacing: 8,
        children: [
          if (EventEnums.payable.contains(status))
            FilledButton.icon(
              onPressed: () => context.push(Routes.eventCheckout(event.id)),
              icon: const Icon(Icons.publish),
              label: Text(status == 'pending_payment' ? 'Complete payment' : 'Pay listing fee and submit'),
            ),
          if (editable)
            OutlinedButton.icon(
              onPressed: () => context.push(Routes.eventEdit(event.id)),
              icon: const Icon(Icons.edit_outlined),
              label: const Text('Edit'),
            ),
          if (status == 'published' || status == 'completed')
            OutlinedButton.icon(
              onPressed: () => context.push(Routes.eventRegistrations(event.id)),
              icon: const Icon(Icons.people_outline),
              label: const Text('Registrations'),
            ),
          if (status == 'published' && !event.summary.isFeatured)
            BusyButton(
              label: 'Feature',
              style: BusyButtonStyle.outlined,
              icon: Icons.star_outline,
              onPressed: () async {
                await ctrl.feature(event.id);
                if (context.mounted) showInfoSnack(context, 'Your event is now featured.');
              },
            ),
          if (EventEnums.cancellable.contains(status))
            BusyButton(
              label: 'Cancel event',
              style: BusyButtonStyle.text,
              onPressed: () async {
                final reason = await promptText(context,
                    title: 'Cancel event', hint: 'Reason shown to registered players', ok: 'Cancel event', required: true);
                if (reason == null) return;
                await ctrl.cancel(event.id, reason);
              },
            ),
          if (event.listing != null)
            Text('Listing fee: ${formatPaise(event.listing!.totalPaise)} (${humanize(event.listing!.status)})'),
        ],
      ),
    );
  }
}

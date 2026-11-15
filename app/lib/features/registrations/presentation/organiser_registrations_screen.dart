import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/format/dates.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/user_tile.dart';
import '../application/registration_controller.dart';
import '../data/registration_api.dart';
import '../data/registration_models.dart';

/// Organiser list of registrations for one event. Payment is only recorded
/// here; Athlix never collects entry fees. Players are identified by name and
/// `qualifiedLabel` only (no documents, no mobile numbers).
class OrganiserRegistrationsScreen extends ConsumerStatefulWidget {
  const OrganiserRegistrationsScreen({super.key, required this.eventId});

  final int eventId;

  @override
  ConsumerState<OrganiserRegistrationsScreen> createState() => _OrganiserRegistrationsScreenState();
}

class _OrganiserRegistrationsScreenState extends ConsumerState<OrganiserRegistrationsScreen> {
  final _list = CursorListController<Registration>();
  String? _status;
  bool _flagged = false;

  Future<void> _apply(Future<Registration> Function() action) async {
    try {
      final updated = await action();
      _list.replaceWhere((r) => r.id == updated.id, (_) => updated);
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  Future<void> _markPayment(Registration r) async {
    final status = await showModalBottomSheet<String>(
      context: context,
      showDragHandle: true,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            for (final s in RegistrationEnums.paymentStatuses)
              ListTile(
                title: Text(humanize(s)),
                trailing: r.paymentStatus == s ? const Icon(Icons.check) : null,
                onTap: () => Navigator.of(ctx).pop(s),
              ),
          ],
        ),
      ),
    );
    if (status == null || status == r.paymentStatus || !mounted) return;
    await _apply(() => ref.read(registrationControllerProvider).markPayment(widget.eventId, r.id, status));
  }

  Future<void> _reject(Registration r) async {
    final ok = await confirm(
      context,
      title: 'Reject this registration?',
      body: r.isFlagged ? 'Their location identity no longer matches this event.' : null,
      ok: 'Reject',
    );
    if (!ok || !mounted) return;
    await _apply(() => ref.read(registrationControllerProvider).reject(widget.eventId, r.id));
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(registrationApiProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Registrations')),
      body: Column(
        children: [
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            child: Row(
              children: [
                FilterChip(
                  avatar: const Icon(Icons.flag_outlined, size: 18),
                  label: const Text('Flagged'),
                  selected: _flagged,
                  onSelected: (v) => setState(() => _flagged = v),
                ),
                const SizedBox(width: 8),
                for (final s in [null, ...RegistrationEnums.statuses])
                  Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: ChoiceChip(
                      label: Text(s == null ? 'All' : humanize(s)),
                      selected: _status == s,
                      onSelected: (_) => setState(() => _status = s),
                    ),
                  ),
              ],
            ),
          ),
          Expanded(
            child: CursorListView<Registration>(
              key: ValueKey('$_status-$_flagged'),
              controller: _list,
              fetch: (cursor) =>
                  api.forEvent(widget.eventId, status: _status, flagged: _flagged, cursor: cursor),
              emptyText: _flagged ? 'No flagged registrations.' : 'No registrations yet.',
              itemBuilder: (context, r, _) => _OrganiserTile(
                reg: r,
                onPayment: () => _markPayment(r),
                onReject: () => _reject(r),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _OrganiserTile extends StatelessWidget {
  const _OrganiserTile({required this.reg, required this.onPayment, required this.onReject});

  final Registration reg;
  final VoidCallback onPayment;
  final VoidCallback onReject;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final user = reg.user;
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (user != null)
              UserTile(user: user, subtitle: reg.qualifiedLabel)
            else
              ListTile(title: const Text('Player'), subtitle: reg.qualifiedLabel == null ? null : Text(reg.qualifiedLabel!)),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (reg.teamName != null) Text('Team: ${reg.teamName}', style: theme.textTheme.bodyMedium),
                  if (reg.teamMembers.isNotEmpty)
                    Text(
                      reg.teamMembers.map((m) => m.role == null ? m.name : '${m.name} (${m.role})').join(', '),
                      style: theme.textTheme.bodySmall,
                    ),
                  const SizedBox(height: 6),
                  Wrap(
                    spacing: 8,
                    runSpacing: 4,
                    children: [
                      Chip(label: Text(humanize(reg.status))),
                      ActionChip(
                        avatar: const Icon(Icons.payments_outlined, size: 18),
                        label: Text(humanize(reg.paymentStatus)),
                        onPressed: reg.isActive ? onPayment : null,
                      ),
                      if (reg.isFlagged)
                        Chip(
                          avatar: const Icon(Icons.flag, size: 18),
                          label: Text('Flagged: ${humanize(reg.eligibilityFlag)}'),
                          backgroundColor: theme.colorScheme.errorContainer,
                          side: BorderSide.none,
                        ),
                    ],
                  ),
                  if (reg.isFlagged && reg.eligibilityFlaggedAt != null)
                    Text(
                      'Flagged ${formatDateTime(reg.eligibilityFlaggedAt)}. '
                      'The registration stays until you decide.',
                      style: theme.textTheme.bodySmall,
                    ),
                  if (reg.isActive)
                    Align(
                      alignment: Alignment.centerRight,
                      child: TextButton(onPressed: onReject, child: const Text('Reject')),
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

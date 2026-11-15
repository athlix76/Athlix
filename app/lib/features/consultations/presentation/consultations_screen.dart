import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/sport_picker.dart';
import '../../../core/widgets/user_avatar.dart';
import '../application/consultation_controller.dart';
import '../data/consultation_api.dart';
import '../data/consultation_models.dart';

/// `/consultations`: my requests as a player and, for coaches, the requests
/// I received. Coaches accept / decline / complete; players cancel pending ones.
class ConsultationsScreen extends ConsumerWidget {
  const ConsultationsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final isCoach = ref.watch(meProvider)?.isCoach ?? false;
    final sides = [if (isCoach) ConsultationSide.coach, ConsultationSide.player];
    return DefaultTabController(
      length: sides.length,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Consultations'),
          actions: [
            if (isCoach)
              TextButton(
                onPressed: () => context.push(Routes.consultationOffer),
                child: const Text('My offer'),
              ),
          ],
          bottom: sides.length > 1
              ? const TabBar(tabs: [Tab(text: 'Received'), Tab(text: 'Sent')])
              : null,
        ),
        body: TabBarView(children: [for (final s in sides) _RequestList(side: s)]),
      ),
    );
  }
}

class _RequestList extends ConsumerStatefulWidget {
  const _RequestList({required this.side});

  final String side;

  @override
  ConsumerState<_RequestList> createState() => _RequestListState();
}

class _RequestListState extends ConsumerState<_RequestList> {
  final _list = CursorListController<ConsultationRequest>();
  String? _status;

  void _replace(ConsultationRequest r) => _list.replaceWhere((x) => x.id == r.id, (_) => r);

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(consultationApiProvider);
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 4),
          child: EnumDropdown(
            label: 'Status',
            values: ConsultationStatus.values,
            value: _status,
            allowAny: true,
            onChanged: (v) => setState(() => _status = v),
          ),
        ),
        Expanded(
          child: CursorListView<ConsultationRequest>(
            key: ValueKey('${widget.side}-$_status'),
            controller: _list,
            fetch: (cursor) => api.mine(side: widget.side, status: _status, cursor: cursor),
            emptyText: widget.side == ConsultationSide.coach
                ? 'No consultation requests yet.'
                : 'You have not requested a consultation yet.',
            itemBuilder: (context, r, _) =>
                _RequestCard(request: r, side: widget.side, onChanged: _replace),
          ),
        ),
      ],
    );
  }
}

class _RequestCard extends ConsumerWidget {
  const _RequestCard({required this.request, required this.side, required this.onChanged});

  final ConsultationRequest request;
  final String side;
  final ValueChanged<ConsultationRequest> onChanged;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final r = request;
    final other = side == ConsultationSide.coach ? r.player : r.coach;
    final ctrl = ref.read(consultationControllerProvider);
    final isCoachSide = side == ConsultationSide.coach;

    Future<void> respond(bool accept) async {
      final note = await promptText(
        context,
        title: accept ? 'Accept request' : 'Decline request',
        hint: 'Note for the player (optional)',
        ok: accept ? 'Accept' : 'Decline',
        required: false,
      );
      if (note == null) return;
      onChanged(await ctrl.respond(r.id, accept: accept, note: note));
    }

    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(0, 4, 0, 12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            ListTile(
              leading: UserAvatar(name: other.displayName, url: other.photoUrl),
              title: Text(other.displayName),
              subtitle: Text([
                ConsultationMode.labels[r.preferredMode] ?? humanize(r.preferredMode),
                timeAgo(r.createdAt),
              ].join(' · ')),
              trailing: Chip(label: Text(humanize(r.status))),
              onTap: () => context.push(Routes.profile(other.id)),
            ),
            if (r.message.isNotEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 16),
                child: Text(r.message),
              ),
            if (r.isAccepted)
              const Padding(
                padding: EdgeInsets.fromLTRB(16, 8, 16, 0),
                child: Text('Accepted: open the profile to message each other. '
                    'Any fee is paid directly to the coach.'),
              ),
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
              child: Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  if (isCoachSide && r.isPending) ...[
                    BusyButton(label: 'Accept', onPressed: () => respond(true)),
                    BusyButton(
                      label: 'Decline',
                      style: BusyButtonStyle.outlined,
                      onPressed: () => respond(false),
                    ),
                  ],
                  if (isCoachSide && r.isAccepted)
                    BusyButton(
                      label: 'Mark completed',
                      onPressed: () async {
                        final ok = await confirm(context, title: 'Mark as completed?', ok: 'Complete');
                        if (ok) onChanged(await ctrl.complete(r.id));
                      },
                    ),
                  if (!isCoachSide && r.isPending)
                    BusyButton(
                      label: 'Cancel request',
                      style: BusyButtonStyle.outlined,
                      onPressed: () async {
                        final ok = await confirm(context, title: 'Cancel this request?', ok: 'Cancel request');
                        if (ok) onChanged(await ctrl.cancel(r.id));
                      },
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

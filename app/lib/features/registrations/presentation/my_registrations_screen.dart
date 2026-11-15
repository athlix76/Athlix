import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../data/registration_api.dart';
import '../data/registration_models.dart';

/// `GET /me/registrations`, filterable by status.
class MyRegistrationsScreen extends ConsumerStatefulWidget {
  const MyRegistrationsScreen({super.key});

  @override
  ConsumerState<MyRegistrationsScreen> createState() => _MyRegistrationsScreenState();
}

class _MyRegistrationsScreenState extends ConsumerState<MyRegistrationsScreen> {
  String? _status;

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(registrationApiProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('My registrations')),
      body: Column(
        children: [
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            child: Row(
              children: [
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
              key: ValueKey(_status),
              fetch: (cursor) => api.mine(status: _status, cursor: cursor),
              emptyText: 'You have not registered for any events yet.',
              itemBuilder: (context, r, _) => _RegistrationTile(reg: r),
            ),
          ),
        ],
      ),
    );
  }
}

class _RegistrationTile extends StatelessWidget {
  const _RegistrationTile({required this.reg});

  final Registration reg;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final ev = reg.event;
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
      child: InkWell(
        onTap: () => context.push(Routes.event(reg.eventId)),
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(ev?.title ?? 'Event', style: theme.textTheme.titleMedium),
              if (ev != null) Text(formatDateTime(ev.startsAt), style: theme.textTheme.bodySmall),
              const SizedBox(height: 8),
              Wrap(
                spacing: 8,
                runSpacing: 4,
                children: [
                  Chip(label: Text(humanize(reg.status))),
                  Chip(label: Text(humanize(reg.paymentStatus))),
                  if (reg.teamName != null) Chip(label: Text(reg.teamName!)),
                ],
              ),
              if (reg.isFlagged && reg.isActive) ...[
                const SizedBox(height: 8),
                MaterialBanner(
                  padding: const EdgeInsets.all(8),
                  backgroundColor: theme.colorScheme.tertiaryContainer,
                  leading: const Icon(Icons.info_outline),
                  content: Text(eligibilityFlagText(reg.eligibilityFlag)),
                  actions: [
                    TextButton(
                      onPressed: () => context.push(Routes.locationIdentity),
                      child: const Text('Location identity'),
                    ),
                  ],
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

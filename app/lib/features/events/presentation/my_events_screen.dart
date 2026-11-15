import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../data/event_api.dart';
import '../data/event_models.dart';
import 'event_card.dart';

/// Events I organise (`GET /me/events`), filterable by status.
class MyEventsScreen extends ConsumerStatefulWidget {
  const MyEventsScreen({super.key});

  @override
  ConsumerState<MyEventsScreen> createState() => _MyEventsScreenState();
}

class _MyEventsScreenState extends ConsumerState<MyEventsScreen> {
  String? _status;

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(eventApiProvider);
    return Scaffold(
      appBar: AppBar(
        title: const Text('My events'),
        actions: [
          IconButton(
            tooltip: 'Organiser verification',
            icon: const Icon(Icons.verified_user_outlined),
            onPressed: () => context.push(Routes.organiserVerification),
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton(
        tooltip: 'Create event',
        onPressed: () => context.push(Routes.eventNew),
        child: const Icon(Icons.add),
      ),
      body: Column(
        children: [
          SizedBox(
            height: 48,
            child: ListView(
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.symmetric(horizontal: 12),
              children: [
                for (final s in <String?>[null, ...EventEnums.myEventStatuses])
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 4),
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
            child: CursorListView<EventSummary>(
              key: ValueKey(_status),
              fetch: (cursor) => api.mine(status: _status, cursor: cursor),
              emptyText: 'You have not created any events yet.',
              itemBuilder: (context, e, _) => EventCard(event: e, showStatus: true),
            ),
          ),
        ],
      ),
    );
  }
}

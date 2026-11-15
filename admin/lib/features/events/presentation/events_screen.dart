import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/format.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/cursor_list.dart';
import '../application/events_providers.dart';
import '../data/events_api.dart';

class EventsScreen extends ConsumerStatefulWidget {
  const EventsScreen({super.key});

  @override
  ConsumerState<EventsScreen> createState() => _EventsScreenState();
}

class _EventsScreenState extends ConsumerState<EventsScreen> {
  final _listKey = GlobalKey<CursorListState<AdminEvent>>();
  final _q = TextEditingController();
  String? _status = 'pending_review';

  @override
  void dispose() {
    _q.dispose();
    super.dispose();
  }

  void _apply() => _listKey.currentState?.reload();

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(eventsApiProvider);
    return PageScaffold(
      title: 'Events',
      subtitle: 'Review events waiting for approval, and remove published events that break the rules.',
      filters: Wrap(spacing: 12, runSpacing: 12, children: [
        FilterTextField(label: 'Title', controller: _q, onSubmitted: (_) => _apply()),
        FilterDropdown(label: 'Status', value: _status, options: eventStatuses, onChanged: (v) {
          setState(() => _status = v);
          _apply();
        }),
      ]),
      body: CursorList<AdminEvent>(
        key: _listKey,
        load: (cursor) => api.list(status: _status, q: _q.text.trim(), cursor: cursor),
        emptyText: 'No events match.',
        itemBuilder: (context, e) => ListTile(
          title: Text('${e.title}  #${e.id}${e.isFeatured ? '  · featured' : ''}'),
          subtitle: Text([
            e.sport?.name ?? '-',
            e.eventType,
            [e.city, e.state].whereType<String>().join(', '),
            'starts ${formatDateTime(e.startsAt)}',
            if (e.organiser != null) 'by ${e.organiser!.label}',
          ].join('  ·  ')),
          trailing: StatusChip(e.status),
          onTap: () => context.go('/events/${e.id}'),
        ),
      ),
    );
  }
}

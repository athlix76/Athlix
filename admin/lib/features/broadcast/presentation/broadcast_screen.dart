import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_error.dart';
import '../../../core/auth/permissions.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../../sports/application/sports_providers.dart';
import '../../sports/data/sports_api.dart';
import '../application/broadcast_providers.dart';
import '../data/broadcast_api.dart';

/// Sends one push + in-app notification to an audience. Minors are excluded
/// unless explicitly included.
class BroadcastScreen extends ConsumerStatefulWidget {
  const BroadcastScreen({super.key});

  @override
  ConsumerState<BroadcastScreen> createState() => _BroadcastScreenState();
}

class _BroadcastScreenState extends ConsumerState<BroadcastScreen> {
  // UI bounds only; the backend enforces the real limits.
  static const _titleMax = 80;
  static const _bodyMax = 300;

  final _title = TextEditingController();
  final _body = TextEditingController();
  final _state = TextEditingController();
  final _roles = <String>{};
  int? _sportId;
  bool _includeMinors = false;
  int? _lastQueued;

  @override
  void dispose() {
    _title.dispose();
    _body.dispose();
    _state.dispose();
    super.dispose();
  }

  String _audienceText(List<AdminSport> sports) {
    final sport = sports.where((s) => s.id == _sportId).map((s) => s.name).firstOrNull;
    return [
      _roles.isEmpty ? 'all roles' : _roles.join(', '),
      sport ?? 'all sports',
      _state.text.trim().isEmpty ? 'all states' : _state.text.trim(),
      _includeMinors ? 'INCLUDING minors' : 'adults only',
    ].join('  ·  ');
  }

  Future<void> _send(List<AdminSport> sports) async {
    final title = _title.text.trim();
    final body = _body.text.trim();
    final queued = await confirmAction<int>(
      context,
      title: 'Send this broadcast?',
      message: 'It cannot be recalled once queued.\n\nAudience: ${_audienceText(sports)}',
      details: KeyValueList([('Title', Text(title)), ('Body', Text(body))]),
      confirmLabel: 'Send',
      destructive: true,
      run: (_) {
        final errors = <String, String>{
          if (title.isEmpty) 'title': 'Required',
          if (title.length > _titleMax) 'title': 'At most $_titleMax characters',
          if (body.isEmpty) 'body': 'Required',
          if (body.length > _bodyMax) 'body': 'At most $_bodyMax characters',
        };
        if (errors.isNotEmpty) throw ApiError.invalidInput(errors);
        return ref.read(broadcastApiProvider).send(
              title: title,
              body: body,
              audience: BroadcastAudience(roles: _roles.toList(), sportId: _sportId, state: _state.text.trim(), includeMinors: _includeMinors),
            );
      },
      successMessage: null,
    );
    if (queued != null && mounted) {
      setState(() => _lastQueued = queued);
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Queued for $queued users')));
    }
  }

  @override
  Widget build(BuildContext context) {
    final sports = ref.watch(adminSportsProvider).valueOrNull ?? const <AdminSport>[];
    final canSend = ref.watch(sessionProvider).can(Perm.notificationsBroadcast);
    return PageScaffold(
      title: 'Broadcast',
      subtitle: 'One notification to many users (category marketing). Users with marketing notifications off do not get the push.',
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 640),
          child: formColumn([
            FormText(label: 'Title (max $_titleMax)', controller: _title),
            FormText(label: 'Body (max $_bodyMax)', controller: _body, maxLines: 4),
            Wrap(spacing: 8, children: [
              const Padding(padding: EdgeInsets.only(top: 8), child: Text('Roles (none = all):')),
              for (final r in broadcastRoles)
                FilterChip(
                  label: Text(r),
                  selected: _roles.contains(r),
                  onSelected: (on) => setState(() => on ? _roles.add(r) : _roles.remove(r)),
                ),
            ]),
            FormDropdown<int>(
              label: 'Sport',
              value: _sportId,
              nullLabel: 'All sports',
              items: [for (final s in sports) (s.id, s.name)],
              onChanged: (v) => setState(() => _sportId = v),
            ),
            FormText(label: 'State (blank = all India)', controller: _state),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('Include minors'),
              subtitle: const Text('Off by default. Only for safety or service notices that minors must see.'),
              value: _includeMinors,
              onChanged: (v) => setState(() => _includeMinors = v),
            ),
            Row(children: [
              FilledButton.icon(
                onPressed: canSend ? () => _send(sports) : null,
                icon: const Icon(Icons.campaign),
                label: const Text('Review and send'),
              ),
              if (_lastQueued != null) ...[
                const SizedBox(width: 16),
                Text('Last broadcast queued for $_lastQueued users.'),
              ],
            ]),
          ]),
        ),
      ),
    );
  }
}

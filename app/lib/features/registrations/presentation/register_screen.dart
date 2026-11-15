import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/models/policy.dart';
import '../../../core/money/money.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/section.dart';
import '../../../core/widgets/simple_markdown.dart';
import '../../events/application/event_providers.dart';
import '../../events/data/event_models.dart';
import '../../location_identity/presentation/not_eligible_sheet.dart';
import '../../policies/application/policy_providers.dart';
import '../application/registration_controller.dart';
import '../data/registration_models.dart';

/// Register for an event. Shows the `entry_fee_disclaimer` policy: the
/// entry fee is paid to the organiser, Athlix never collects it.
class RegisterScreen extends ConsumerWidget {
  const RegisterScreen({super.key, required this.eventId});

  final int eventId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(eventProvider(eventId));
    return Scaffold(
      appBar: AppBar(title: const Text('Register')),
      body: AsyncValueView<Event>(
        value: value,
        onRetry: () => ref.invalidate(eventProvider(eventId)),
        data: (e) => _RegisterForm(event: e),
      ),
    );
  }
}

class _RegisterForm extends ConsumerStatefulWidget {
  const _RegisterForm({required this.event});

  final Event event;

  @override
  ConsumerState<_RegisterForm> createState() => _RegisterFormState();
}

class _RegisterFormState extends ConsumerState<_RegisterForm> {
  final _form = GlobalKey<FormState>();
  final _teamName = TextEditingController();
  final _members = <(TextEditingController, TextEditingController)>[];
  bool _ack = false;

  bool get _isTeam => widget.event.summary.isTeam;

  @override
  void initState() {
    super.initState();
    if (_isTeam) {
      final min = widget.event.teamSizeMin ?? 1;
      for (var i = 0; i < min.clamp(1, RegistrationEnums.maxTeamMembers); i++) {
        _addMember();
      }
    }
  }

  @override
  void dispose() {
    _teamName.dispose();
    for (final (a, b) in _members) {
      a.dispose();
      b.dispose();
    }
    super.dispose();
  }

  void _addMember() => _members.add((TextEditingController(), TextEditingController()));

  int get _maxMembers {
    final max = widget.event.teamSizeMax ?? RegistrationEnums.maxTeamMembers;
    return max.clamp(1, RegistrationEnums.maxTeamMembers);
  }

  Future<void> _submit() async {
    if (!(_form.currentState?.validate() ?? false)) return;
    final members = [
      for (final (name, role) in _members)
        if (name.text.trim().isNotEmpty) TeamMember(name: name.text.trim(), role: role.text.trim()),
    ];
    try {
      final r = await ref.read(registrationControllerProvider).register(
            widget.event.id,
            teamName: _isTeam ? _teamName.text.trim() : null,
            members: _isTeam ? members : null,
          );
      if (!mounted) return;
      showInfoSnack(
        context,
        r.status == 'waitlisted' ? 'You are on the waitlist.' : 'Registered. Pay the organiser as they ask.',
      );
      context.pop();
    } catch (e) {
      if (!mounted) return;
      if (await showNotEligibleSheetIfNeeded(context, e)) return;
      rethrow;
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = widget.event.summary;
    final disclaimer = ref.watch(policyDocumentProvider(PolicyDocType.entryFeeDisclaimer));
    return Form(
      key: _form,
      child: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Text(s.title, style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 4),
          Text('Entry fee: ${formatEntryFee(s.entryFeePaise)}'),
          if (s.slotsLeft <= 0) const Text('The event is full. You will join the waitlist.'),
          if (_isTeam) ...[
            const SectionHeader('Team'),
            TextFormField(
              controller: _teamName,
              decoration: const InputDecoration(labelText: 'Team name'),
              validator: (v) => (v == null || v.trim().isEmpty) ? 'Required' : null,
            ),
            const SizedBox(height: 8),
            Text(
              'Team members are names only and are not verified. '
              '${widget.event.teamSizeMin ?? 1} to $_maxMembers players.',
              style: Theme.of(context).textTheme.bodySmall,
            ),
            for (var i = 0; i < _members.length; i++)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Row(
                  children: [
                    Expanded(
                      flex: 3,
                      child: TextFormField(
                        controller: _members[i].$1,
                        decoration: InputDecoration(labelText: 'Player ${i + 1}'),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      flex: 2,
                      child: TextFormField(
                        controller: _members[i].$2,
                        decoration: const InputDecoration(labelText: 'Role (optional)'),
                      ),
                    ),
                    IconButton(
                      tooltip: 'Remove',
                      icon: const Icon(Icons.remove_circle_outline),
                      onPressed: _members.length <= 1
                          ? null
                          : () => setState(() {
                                final (a, b) = _members.removeAt(i);
                                a.dispose();
                                b.dispose();
                              }),
                    ),
                  ],
                ),
              ),
            if (_members.length < _maxMembers)
              Align(
                alignment: Alignment.centerLeft,
                child: TextButton.icon(
                  onPressed: () => setState(_addMember),
                  icon: const Icon(Icons.add),
                  label: const Text('Add player'),
                ),
              ),
          ],
          const SectionHeader('Entry fee'),
          disclaimer.when(
            data: (d) => SimpleMarkdown(d.contentMd),
            loading: () => const LinearProgressIndicator(),
            error: (_, __) => const Text(
              'The entry fee is paid to the organiser directly. Athlix does not collect or refund it.',
            ),
          ),
          CheckboxListTile(
            contentPadding: EdgeInsets.zero,
            value: _ack,
            onChanged: (v) => setState(() => _ack = v ?? false),
            title: const Text('I understand the entry fee is paid to the organiser, not to Athlix.'),
          ),
          const SizedBox(height: 16),
          BusyButton(
            label: s.slotsLeft > 0 ? 'Register' : 'Join waitlist',
            onPressed: _ack ? _submit : null,
          ),
        ],
      ),
    );
  }
}

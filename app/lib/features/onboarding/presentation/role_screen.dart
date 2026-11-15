import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/session/session_controller.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/sport_picker.dart';
import '../application/onboarding_controller.dart';
import '../data/onboarding_api.dart';

/// Onboarding step `role`.
class RoleScreen extends ConsumerStatefulWidget {
  const RoleScreen({super.key});

  @override
  ConsumerState<RoleScreen> createState() => _RoleScreenState();
}

class _RoleScreenState extends ConsumerState<RoleScreen> {
  String _role = UserRole.player;
  final Set<String> _extra = {};

  static const _descriptions = {
    UserRole.player: 'Find events, register and track your stats',
    UserRole.coach: 'Show your coaching profile and offer consultations',
    UserRole.organiser: 'List tournaments, trials and camps',
  };

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Who are you?'),
        actions: [
          TextButton(
            onPressed: () => ref.read(sessionProvider.notifier).logout(),
            child: const Text('Sign out'),
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          for (final r in UserRole.all)
            Card(
              child: RadioListTile<String>(
                value: r,
                groupValue: _role,
                onChanged: (v) => setState(() {
                  _role = v ?? UserRole.player;
                  _extra.remove(_role);
                }),
                title: Text(EnumDropdown.pretty(r)),
                subtitle: Text(_descriptions[r] ?? ''),
              ),
            ),
          const SizedBox(height: 12),
          const Text('Also a...'),
          Wrap(
            spacing: 8,
            children: [
              for (final r in UserRole.all)
                if (r != _role)
                  FilterChip(
                    label: Text(EnumDropdown.pretty(r)),
                    selected: _extra.contains(r),
                    onSelected: (on) => setState(() => on ? _extra.add(r) : _extra.remove(r)),
                  ),
            ],
          ),
          const SizedBox(height: 24),
          BusyButton(
            label: 'Continue',
            onPressed: () =>
                ref.read(onboardingControllerProvider).chooseRole(_role, _extra.toList()),
          ),
        ],
      ),
    );
  }
}

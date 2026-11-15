import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/widgets/common.dart';
import '../../../core/widgets/cursor_list.dart';
import '../application/users_providers.dart';
import '../data/users_api.dart';

/// User search. `q` = display name prefix, `#42`, or an exact mobile number
/// (the backend looks it up by hash; the number is never shown back).
class UsersScreen extends ConsumerStatefulWidget {
  const UsersScreen({super.key});

  @override
  ConsumerState<UsersScreen> createState() => _UsersScreenState();
}

class _UsersScreenState extends ConsumerState<UsersScreen> {
  final _listKey = GlobalKey<CursorListState<AdminUserRow>>();
  final _q = TextEditingController();
  String? _role;
  String? _status;
  String? _verification;
  String? _minor;

  @override
  void dispose() {
    _q.dispose();
    super.dispose();
  }

  UserFilters get _filters => UserFilters(
        q: _q.text.trim(),
        role: _role,
        status: _status,
        verificationStatus: _verification,
        isMinor: _minor == null ? null : _minor == 'minor',
      );

  void _apply() => _listKey.currentState?.reload();

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(usersApiProvider);
    return PageScaffold(
      title: 'Users',
      subtitle: 'Search by name prefix, #id or exact mobile number. Mobile numbers are shown masked.',
      filters: Wrap(spacing: 12, runSpacing: 12, children: [
        FilterTextField(label: 'Search', controller: _q, onSubmitted: (_) => _apply(), width: 300),
        FilterDropdown(label: 'Role', value: _role, options: const ['player', 'coach', 'organiser'], onChanged: (v) {
          setState(() => _role = v);
          _apply();
        }),
        FilterDropdown(label: 'Status', value: _status, options: const ['active', 'suspended', 'deleted'], onChanged: (v) {
          setState(() => _status = v);
          _apply();
        }),
        FilterDropdown(
            label: 'Verification',
            value: _verification,
            options: const ['unverified', 'pending', 'verified', 'failed'],
            onChanged: (v) {
              setState(() => _verification = v);
              _apply();
            }),
        FilterDropdown(label: 'Age group', value: _minor, options: const ['minor', 'adult'], onChanged: (v) {
          setState(() => _minor = v);
          _apply();
        }),
      ]),
      body: CursorList<AdminUserRow>(
        key: _listKey,
        load: (cursor) => api.search(_filters, cursor),
        emptyText: 'No users match.',
        itemBuilder: (context, u) => ListTile(
          title: Text('${u.displayName}  #${u.id}'),
          subtitle: Text([
            u.mobileMasked,
            u.roles.join(', '),
            'verification: ${u.verificationStatus}',
            'onboarding: ${u.onboardingStep}',
            if (u.isMinor) 'minor (guardian: ${u.guardianConsentStatus})',
          ].join('  ·  ')),
          trailing: StatusChip(u.status),
          onTap: () => context.go('/users/${u.id}'),
        ),
      ),
    );
  }
}

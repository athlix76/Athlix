import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/json.dart';
import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/permission_gate.dart';
import '../application/users_providers.dart';
import '../data/users_api.dart';

class UserDetailScreen extends ConsumerStatefulWidget {
  const UserDetailScreen({super.key, required this.userId});

  final int userId;

  @override
  ConsumerState<UserDetailScreen> createState() => _UserDetailScreenState();
}

class _UserDetailScreenState extends ConsumerState<UserDetailScreen> {
  String? _verifiedName;

  @override
  void didUpdateWidget(covariant UserDetailScreen oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.userId != widget.userId) _verifiedName = null;
  }

  Future<void> _reveal() async {
    final api = ref.read(usersApiProvider);
    final name = await confirmAction<String>(
      context,
      title: 'Reveal verified name?',
      message: 'The name from DigiLocker is personal data. Revealing it is recorded in the audit log with your admin id. '
          'Only reveal it when you need it for this case.',
      confirmLabel: 'Reveal',
      run: (_) async => (await api.revealVerifiedName(widget.userId)) ?? '(no verified name)',
    );
    if (name != null && mounted) setState(() => _verifiedName = name);
  }

  Future<void> _suspend(AdminUserDetail d) async {
    final api = ref.read(usersApiProvider);
    final r = await confirmAction<AdminUserRow>(
      context,
      title: 'Suspend ${d.row.displayName}?',
      message: 'The user is signed out and cannot use Athlix until restored.',
      confirmLabel: 'Suspend',
      destructive: true,
      reason: const ReasonSpec(hint: 'Shown to other admins in the audit log'),
      run: (reason) => api.suspend(widget.userId, reason!),
      successMessage: 'User suspended',
    );
    if (r != null) ref.invalidate(userDetailProvider(widget.userId));
  }

  Future<void> _revokeGuardian(AdminUserDetail d) async {
    final api = ref.read(usersApiProvider);
    final r = await confirmAction<String>(
      context,
      title: 'Withdraw guardian consent for ${d.row.displayName}?',
      message: 'The minor cannot use Athlix until a guardian gives consent again. '
          'Use this when the guardian asks support to withdraw consent.',
      confirmLabel: 'Withdraw consent',
      destructive: true,
      reason: const ReasonSpec(hint: 'Up to 255 characters, kept in the consent record and audit log'),
      run: (reason) => api.revokeGuardianConsent(widget.userId, reason!),
      successMessage: 'Guardian consent withdrawn',
    );
    if (r != null) ref.invalidate(userDetailProvider(widget.userId));
  }

  Future<void> _restore(AdminUserDetail d) async {
    final api = ref.read(usersApiProvider);
    final r = await confirmAction<AdminUserRow>(
      context,
      title: 'Restore ${d.row.displayName}?',
      message: 'The user can sign in again.',
      confirmLabel: 'Restore',
      run: (_) => api.unsuspend(widget.userId),
      successMessage: 'User restored',
    );
    if (r != null) ref.invalidate(userDetailProvider(widget.userId));
  }

  @override
  Widget build(BuildContext context) {
    final value = ref.watch(userDetailProvider(widget.userId));
    return PageScaffold(
      title: 'User #${widget.userId}',
      actions: [
        IconButton(
          tooltip: 'Reload',
          icon: const Icon(Icons.refresh),
          onPressed: () => ref.invalidate(userDetailProvider(widget.userId)),
        ),
      ],
      body: AsyncBody<AdminUserDetail>(
        value: value,
        onRetry: () => ref.invalidate(userDetailProvider(widget.userId)),
        data: (d) {
          final u = d.row;
          final v = d.verification;
          final g = d.guardian;
          return SingleChildScrollView(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              SectionCard(
                title: u.displayName,
                actions: [
                  StatusChip(u.status),
                  const SizedBox(width: 8),
                  IfCan(
                    Perm.usersWrite,
                    child: u.status == 'suspended'
                        ? OutlinedButton(onPressed: () => _restore(d), child: const Text('Restore'))
                        : u.status == 'active'
                            ? OutlinedButton(onPressed: () => _suspend(d), child: const Text('Suspend'))
                            : const SizedBox.shrink(),
                  ),
                  if (u.isMinor && u.guardianConsentStatus == 'granted') ...[
                    const SizedBox(width: 8),
                    IfCan(
                      Perm.usersWrite,
                      child: OutlinedButton(
                        onPressed: () => _revokeGuardian(d),
                        child: const Text('Withdraw guardian consent'),
                      ),
                    ),
                  ],
                ],
                child: KeyValueList([
                  ('Mobile', textValue(u.mobileMasked)),
                  ('Roles', textValue(u.roles.join(', '))),
                  ('Primary role', textValue(u.primaryRole)),
                  ('Onboarding step', textValue(u.onboardingStep)),
                  ('Minor', textValue(u.isMinor ? 'yes' : 'no')),
                  ('Guardian consent', textValue(u.guardianConsentStatus)),
                  ('Joined', textValue(formatDateTime(u.createdAt))),
                  ('Last login', textValue(formatDateTime(u.lastLoginAt))),
                  if (d.suspendedReason != null) ('Suspended because', textValue(d.suspendedReason)),
                ]),
              ),
              SectionCard(
                title: 'Identity verification',
                child: v == null
                    ? const Text('Not verified.')
                    : KeyValueList([
                        ('Status', textValue(v['status'])),
                        ('Provider', textValue(v['provider'])),
                        ('Aadhaar', textValue(v['aadhaarLast4'] == null ? null : 'XXXX XXXX ${v['aadhaarLast4']}')),
                        ('Verified at', textValue(formatDateTime(asDate(v['verifiedAt'])))),
                        (
                          'Verified name',
                          _verifiedName != null
                              ? SelectableText(_verifiedName!)
                              : Align(
                                  alignment: Alignment.centerLeft,
                                  child: TextButton.icon(
                                    onPressed: _reveal,
                                    icon: const Icon(Icons.visibility_outlined, size: 18),
                                    label: const Text('Reveal (audited)'),
                                  ),
                                ),
                        ),
                      ]),
              ),
              if (g != null)
                SectionCard(
                  title: 'Guardian',
                  child: KeyValueList([
                    ('Relation', textValue(g['relation'])),
                    ('Mobile', textValue(g['mobileMasked'])),
                    ('Consent', textValue(g['status'])),
                    ('Granted at', textValue(formatDateTime(asDate(g['grantedAt'])))),
                  ]),
                ),
              SectionCard(
                title: 'Activity',
                child: KeyValueList([
                  ('Events organised', textValue(d.counts['eventsOrganised'])),
                  ('Registrations', textValue(d.counts['registrations'])),
                  ('Posts', textValue(d.counts['posts'])),
                  ('Reports against', textValue(d.counts['reportsAgainst'])),
                ]),
              ),
            ]),
          );
        },
      ),
    );
  }
}

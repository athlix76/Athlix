import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/format/dates.dart';
import '../../../core/models/policy.dart';
import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/section.dart';
import '../../guardian/data/guardian_api.dart';
import '../application/settings_controller.dart';
import '../data/settings_models.dart';

/// Policies listed in settings, in this order (every [PolicyDocType] value).
const _policyDocTypes = [
  PolicyDocType.terms,
  PolicyDocType.privacy,
  PolicyDocType.communityGuidelines,
  PolicyDocType.listingRefund,
  PolicyDocType.entryFeeDisclaimer,
  PolicyDocType.organiserTerms,
  PolicyDocType.guardianConsent,
  PolicyDocType.locationIdentityNotice,
];

/// `/settings`: push categories, privacy, account and legal.
class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final settings = ref.watch(settingsControllerProvider);
    final config = ref.watch(appConfigProvider);
    final me = ref.watch(meProvider);
    final version = ref.watch(appVersionProvider).valueOrNull;

    return Scaffold(
      appBar: AppBar(title: const Text('Settings')),
      body: ListView(
        children: [
          AsyncValueView<Settings>(
            value: settings,
            onRetry: () => ref.invalidate(settingsControllerProvider),
            data: (s) => _SettingsSections(settings: s),
          ),
          const SectionHeader('Account'),
          if (config.locationEnabled)
            ListTile(
              leading: const Icon(Icons.place_outlined),
              title: const Text('Location identity'),
              subtitle: const Text('Native or residence district for eligible events'),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => context.push(Routes.locationIdentity),
            ),
          ListTile(
            leading: const Icon(Icons.block),
            title: const Text('Blocked users'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => context.push(Routes.blocked),
          ),
          if (me != null) _AddRoleTile(roles: {...me.roles, if (me.primaryRole != null) me.primaryRole!}),
          if (me?.isOrganiser ?? false)
            ListTile(
              leading: const Icon(Icons.verified_user_outlined),
              title: const Text('Organiser verification'),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => context.push(Routes.organiserVerification),
            ),
          if (me?.isCoach ?? false)
            ListTile(
              leading: const Icon(Icons.sports),
              title: const Text('My consultation offer'),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => context.push(Routes.consultationOffer),
            ),
          if (me != null && me.isMinor && me.guardianConsentStatus == 'granted')
            ListTile(
              leading: const Icon(Icons.family_restroom),
              title: const Text('Withdraw guardian consent'),
              subtitle: const Text('Your guardian confirms with a code'),
              onTap: () => _withdrawGuardianConsent(context, ref),
            ),
          ListTile(
            leading: const Icon(Icons.forum_outlined),
            title: const Text('Consultation requests'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => context.push(Routes.consultations),
          ),
          if (config.premiumEnabled)
            ListTile(
              leading: const Icon(Icons.workspace_premium_outlined),
              title: const Text('Premium'),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => context.push(Routes.premium),
            ),
          const SectionHeader('Legal'),
          for (final docType in _policyDocTypes)
            ListTile(
              leading: const Icon(Icons.description_outlined),
              title: Text(humanize(docType)),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => context.push(Routes.policy(docType)),
            ),
          if ((config.supportEmail ?? '').isNotEmpty)
            InfoRow(label: 'Support', value: config.supportEmail!, icon: Icons.support_agent),
          if ((config.grievanceOfficerName ?? '').isNotEmpty)
            InfoRow(
              label: 'Grievance officer',
              value: [config.grievanceOfficerName, config.grievanceOfficerEmail]
                  .whereType<String>()
                  .where((s) => s.isNotEmpty)
                  .join(' · '),
              icon: Icons.gavel_outlined,
            ),
          const SectionHeader('Session'),
          ListTile(
            leading: const Icon(Icons.logout),
            title: const Text('Log out'),
            onTap: () => _logout(context, ref),
          ),
          ListTile(
            leading: Icon(Icons.delete_forever_outlined, color: Theme.of(context).colorScheme.error),
            title: Text('Delete account', style: TextStyle(color: Theme.of(context).colorScheme.error)),
            onTap: () => _deleteAccount(context, ref),
          ),
          ListTile(
            dense: true,
            title: Text(
              version == null ? 'Athlix' : 'Athlix $version',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ),
          const SizedBox(height: 24),
        ],
      ),
    );
  }

  Future<void> _logout(BuildContext context, WidgetRef ref) async {
    final ok = await confirm(context, title: 'Log out?', ok: 'Log out');
    if (!ok) return;
    try {
      await ref.read(accountActionsProvider).logout();
    } catch (e) {
      if (context.mounted) showErrorSnack(context, e);
    }
  }

  /// Guardian withdraws consent: a code goes to the guardian, the user enters
  /// it, and the session's Me moves back to the guardian onboarding step (the
  /// router then shows the guardian screen).
  Future<void> _withdrawGuardianConsent(BuildContext context, WidgetRef ref) async {
    final ok = await confirm(
      context,
      title: 'Withdraw guardian consent?',
      body: 'A code is sent to your guardian. After they confirm, you cannot use Athlix '
          'until a guardian gives consent again.',
      ok: 'Send code',
    );
    if (!ok || !context.mounted) return;
    final api = ref.read(guardianApiProvider);
    try {
      final challenge = await api.withdrawRequest();
      if (!context.mounted) return;
      final otp = await promptText(
        context,
        title: 'Guardian code',
        hint: 'Code sent to ${challenge.guardianMobileMasked}',
        ok: 'Withdraw consent',
        maxLength: 8,
      );
      if (otp == null || otp.isEmpty) return;
      final me = await api.withdrawVerify(challenge.consentId, otp);
      ref.read(sessionProvider.notifier).setMe(me);
    } catch (e) {
      if (context.mounted) showErrorSnack(context, e);
    }
  }

  Future<void> _deleteAccount(BuildContext context, WidgetRef ref) async {
    final ok = await confirm(
      context,
      title: 'Delete your account?',
      body: 'Your profile, posts and messages are hidden and you are signed out on every '
          'device. This cannot be undone.',
      ok: 'Delete',
    );
    if (!ok) return;
    try {
      await ref.read(accountActionsProvider).deleteAccount();
    } catch (e) {
      if (context.mounted) showErrorSnack(context, e);
    }
  }
}

class _SettingsSections extends ConsumerWidget {
  const _SettingsSections({required this.settings});

  final Settings settings;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final ctrl = ref.read(settingsControllerProvider.notifier);
    final p = settings.notificationPrefs;
    final privacy = settings.privacy;

    Future<void> run(Future<void> Function() action) async {
      try {
        await action();
      } catch (e) {
        if (context.mounted) showErrorSnack(context, e);
      }
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SectionHeader('Push notifications'),
        SwitchListTile(
          title: const Text('Messages'),
          value: p.messages,
          onChanged: (v) => run(() => ctrl.setNotificationPrefs(p.copyWith(messages: v))),
        ),
        SwitchListTile(
          title: const Text('Events and registrations'),
          value: p.events,
          onChanged: (v) => run(() => ctrl.setNotificationPrefs(p.copyWith(events: v))),
        ),
        SwitchListTile(
          title: const Text('Follows, posts, communities and consultations'),
          value: p.social,
          onChanged: (v) => run(() => ctrl.setNotificationPrefs(p.copyWith(social: v))),
        ),
        SwitchListTile(
          title: const Text('News and offers'),
          value: p.marketing,
          onChanged: (v) => run(() => ctrl.setNotificationPrefs(p.copyWith(marketing: v))),
        ),
        const SectionHeader('Privacy'),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          child: DropdownButtonFormField<String>(
            value: AllowDmsFrom.values.contains(privacy.allowDmsFrom) ? privacy.allowDmsFrom : null,
            isExpanded: true,
            decoration: const InputDecoration(labelText: 'Who can message me'),
            items: [
              for (final v in AllowDmsFrom.values)
                DropdownMenuItem(value: v, child: Text(AllowDmsFrom.labels[v] ?? humanize(v))),
            ],
            onChanged: (v) {
              if (v == null || v == privacy.allowDmsFrom) return;
              run(() => ctrl.setPrivacy(privacy.copyWith(allowDmsFrom: v)));
            },
          ),
        ),
        SwitchListTile(
          title: const Text('Show me in search'),
          value: privacy.showInSearch,
          onChanged: (v) => run(() => ctrl.setPrivacy(privacy.copyWith(showInSearch: v))),
        ),
      ],
    );
  }
}

/// "Add a role": offers the roles the user does not have yet.
class _AddRoleTile extends ConsumerWidget {
  const _AddRoleTile({required this.roles});

  final Set<String> roles;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final missing = AddableRole.values.where((r) => !roles.contains(r)).toList();
    if (missing.isEmpty) return const SizedBox.shrink();
    return ListTile(
      leading: const Icon(Icons.person_add_alt_1_outlined),
      title: const Text('Add a role'),
      subtitle: Text('You are: ${roles.map(humanize).join(', ')}'),
      trailing: const Icon(Icons.chevron_right),
      onTap: () => _pick(context, ref, missing),
    );
  }

  Future<void> _pick(BuildContext context, WidgetRef ref, List<String> missing) async {
    final role = await showModalBottomSheet<String>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const ListTile(title: Text('Add a role')),
            for (final r in missing)
              ListTile(title: Text(humanize(r)), onTap: () => Navigator.of(ctx).pop(r)),
          ],
        ),
      ),
    );
    if (role == null || !context.mounted) return;
    try {
      await ref.read(accountActionsProvider).addRole(role);
      if (!context.mounted) return;
      if (role == 'organiser') {
        showInfoSnack(context, 'Organiser role added. Complete organiser verification to list events.');
        context.push(Routes.organiserVerification);
      } else {
        showInfoSnack(context, '${humanize(role)} role added.');
      }
    } catch (e) {
      if (context.mounted) showErrorSnack(context, e);
    }
  }
}

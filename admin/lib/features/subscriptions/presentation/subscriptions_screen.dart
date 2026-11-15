import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/cursor_list.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../auth/application/session_controller.dart';
import '../../plans/application/plans_providers.dart';
import '../../plans/data/plans_api.dart';
import '../application/subscriptions_providers.dart';
import '../data/subscriptions_api.dart';

class SubscriptionsScreen extends ConsumerStatefulWidget {
  const SubscriptionsScreen({super.key});

  @override
  ConsumerState<SubscriptionsScreen> createState() => _SubscriptionsScreenState();
}

class _SubscriptionsScreenState extends ConsumerState<SubscriptionsScreen> {
  final _listKey = GlobalKey<CursorListState<AdminSubscription>>();
  final _userId = TextEditingController();
  String? _status;
  int? _planId;

  @override
  void dispose() {
    _userId.dispose();
    super.dispose();
  }

  void _apply() => _listKey.currentState?.reload();

  Future<void> _grant(List<Plan> plans) async {
    final api = ref.read(subscriptionsApiProvider);
    final userId = TextEditingController();
    final days = TextEditingController(text: '30');
    final reason = TextEditingController();
    int? planId;
    final ok = await showEditorDialog(
      context,
      title: 'Grant a subscription',
      builder: (_, setState) => formColumn([
        const Text('Gives a user a plan without payment (store "manual"), for support cases and partners.'),
        FormText(label: 'User id', controller: userId),
        FormDropdown<int>(
          label: 'Plan',
          value: planId,
          items: [for (final p in plans) (p.id!, '${p.name} (${p.audience})')],
          onChanged: (v) => setState(() => planId = v),
        ),
        FormText(label: 'Days', controller: days),
        FormText(label: 'Reason (kept in the audit log)', controller: reason, maxLines: 3),
      ]),
      validate: () => {
        if (int.tryParse(userId.text.trim()) == null) 'userId': 'a user id number',
        if (planId == null) 'plan': 'pick a plan',
        if ((int.tryParse(days.text.trim()) ?? 0) <= 0) 'days': 'a whole number above 0',
        if (reason.text.trim().isEmpty) 'reason': 'is required',
      },
      confirmTitle: 'Grant subscription?',
      confirmMessage: 'The user gets the plan benefits straight away, free of charge.',
      saveLabel: 'Grant',
      successMessage: 'Subscription granted',
      save: () => api.grant(
        userId: int.parse(userId.text.trim()),
        planId: planId!,
        days: int.parse(days.text.trim()),
        reason: reason.text.trim(),
      ),
    );
    if (ok) _apply();
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(subscriptionsApiProvider);
    final plans = ref.watch(plansProvider).valueOrNull ?? const <Plan>[];
    final canGrant = ref.watch(sessionProvider).can(Perm.plansWrite);
    return PageScaffold(
      title: 'Subscriptions',
      actions: [
        if (canGrant)
          FilledButton.icon(
            onPressed: plans.isEmpty ? null : () => _grant(plans),
            icon: const Icon(Icons.card_giftcard),
            label: const Text('Grant'),
          ),
      ],
      filters: Wrap(spacing: 12, runSpacing: 12, children: [
        FilterTextField(label: 'User id', controller: _userId, onSubmitted: (_) => _apply(), width: 160),
        FilterDropdown(label: 'Status', value: _status, options: subscriptionStatuses, onChanged: (v) {
          setState(() => _status = v);
          _apply();
        }),
        SizedBox(
          width: 240,
          child: FormDropdown<int>(
            label: 'Plan',
            value: _planId,
            nullLabel: 'Any',
            items: [for (final p in plans) (p.id!, p.name)],
            onChanged: (v) {
              setState(() => _planId = v);
              _apply();
            },
          ),
        ),
      ]),
      body: CursorList<AdminSubscription>(
        key: _listKey,
        load: (cursor) => api.list(userId: int.tryParse(_userId.text.trim()), status: _status, planId: _planId, cursor: cursor),
        emptyText: 'No subscriptions match.',
        itemBuilder: (context, s) => ListTile(
          title: Text('${s.planName}  ·  ${s.user?.label ?? '-'}'),
          subtitle: Text('${s.store}  ·  started ${formatDateTime(s.startedAt)}  ·  expires ${formatDateTime(s.expiresAt)}'
              '${s.autoRenew ? '  ·  auto-renew' : ''}'),
          trailing: StatusChip(s.status),
          onTap: s.user == null ? null : () => context.go('/users/${s.user!.id}'),
        ),
      ),
    );
  }
}

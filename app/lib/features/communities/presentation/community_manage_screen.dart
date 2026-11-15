import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/models/user_summary.dart';
import '../../../core/router/routes.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/user_tile.dart';
import '../../chat/data/chat_api.dart';
import '../../chat/data/chat_models.dart';
import '../application/community_providers.dart';
import '../data/community_api.dart';
import '../data/community_models.dart';

/// Owner/admin tools: members (roles, remove, ban), join requests, invites.
/// The server enforces every permission; the UI only hides what can't work.
class CommunityManageScreen extends ConsumerWidget {
  const CommunityManageScreen({super.key, required this.communityId});

  final int communityId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(communityProvider(communityId));
    return AsyncValueView<Community>(
      value: value,
      onRetry: () => ref.invalidate(communityProvider(communityId)),
      data: (c) => DefaultTabController(
        length: 3,
        child: Scaffold(
          appBar: AppBar(
            title: Text(c.title),
            bottom: const TabBar(tabs: [Tab(text: 'Members'), Tab(text: 'Requests'), Tab(text: 'Invite')]),
          ),
          body: c.canModerate
              ? TabBarView(
                  children: [
                    _MembersTab(community: c),
                    _RequestsTab(community: c),
                    _InviteTab(community: c),
                  ],
                )
              : const EmptyView(text: 'Only the owner and admins can manage this community.', icon: Icons.lock_outline),
        ),
      ),
    );
  }
}

class _MembersTab extends ConsumerStatefulWidget {
  const _MembersTab({required this.community});

  final Community community;

  @override
  ConsumerState<_MembersTab> createState() => _MembersTabState();
}

class _MembersTabState extends ConsumerState<_MembersTab> {
  final _list = CursorListController<ConversationMember>();

  Community get _c => widget.community;

  Future<void> _act(ConversationMember m, String action) async {
    final ctl = ref.read(communityControllerProvider);
    final name = m.user.displayName;
    try {
      switch (action) {
        case 'admin':
        case 'member':
          await ctl.setRole(_c.id, m.user.id, action);
          _list.replaceWhere(
            (x) => x.user.id == m.user.id,
            (x) => ConversationMember(user: x.user, role: action, joinedAt: x.joinedAt),
          );
          if (mounted) showInfoSnack(context, action == 'admin' ? '$name is now an admin.' : '$name is now a member.');
        case 'owner':
          if (!await confirm(context,
              title: 'Make $name the owner?',
              body: 'You will become an admin and can no longer change roles or transfer the community.',
              ok: 'Transfer')) {
            return;
          }
          await ctl.transferOwnership(_c.id, m.user.id);
          if (mounted) {
            showInfoSnack(context, '$name now owns this community.');
            Navigator.of(context).maybePop();
          }
        case 'remove':
          if (!await confirm(context, title: 'Remove $name?', body: 'They can join again later.', ok: 'Remove')) return;
          await ctl.removeMember(_c.id, m.user.id);
          _list.removeWhere((x) => x.user.id == m.user.id);
        case 'ban':
          if (!await confirm(context, title: 'Ban $name?', body: "They will be removed and can't rejoin.", ok: 'Ban')) {
            return;
          }
          await ctl.banMember(_c.id, m.user.id);
          _list.removeWhere((x) => x.user.id == m.user.id);
        case 'profile':
          await context.push(Routes.profile(m.user.id));
      }
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  @override
  Widget build(BuildContext context) {
    final me = ref.watch(currentUserIdProvider);
    return CursorListView<ConversationMember>(
      controller: _list,
      fetch: (cursor) => ref.read(chatApiProvider).members(_c.id, cursor: cursor),
      emptyText: 'No members.',
      itemBuilder: (context, m, _) {
        final isMe = m.user.id == me;
        final target = m.role;
        // Owner: anything on anyone but self. Admin: remove/ban plain members only.
        final canRole = _c.isOwner && !isMe && target != MemberRole.owner;
        final canKick = !isMe && target != MemberRole.owner && (_c.isOwner || target == MemberRole.member);
        return UserTile(
          user: m.user,
          subtitle: humanize(m.role),
          onTap: () => _act(m, 'profile'),
          trailing: canRole || canKick
              ? PopupMenuButton<String>(
                  onSelected: (a) => _act(m, a),
                  itemBuilder: (_) => [
                    if (canRole && target != MemberRole.admin) const PopupMenuItem(value: 'admin', child: Text('Make admin')),
                    if (canRole && target == MemberRole.admin)
                      const PopupMenuItem(value: 'member', child: Text('Remove admin role')),
                    if (canRole && target == MemberRole.admin)
                      const PopupMenuItem(value: 'owner', child: Text('Make owner')),
                    if (canKick) const PopupMenuItem(value: 'remove', child: Text('Remove')),
                    if (canKick) const PopupMenuItem(value: 'ban', child: Text('Ban')),
                  ],
                )
              : null,
        );
      },
    );
  }
}

class _RequestsTab extends ConsumerStatefulWidget {
  const _RequestsTab({required this.community});

  final Community community;

  @override
  ConsumerState<_RequestsTab> createState() => _RequestsTabState();
}

class _RequestsTabState extends ConsumerState<_RequestsTab> {
  final _list = CursorListController<JoinRequest>();

  Future<void> _decide(JoinRequest r, bool approve) async {
    final ctl = ref.read(communityControllerProvider);
    try {
      if (approve) {
        await ctl.approve(widget.community.id, r.user.id);
      } else {
        await ctl.reject(widget.community.id, r.user.id);
      }
      _list.removeWhere((x) => x.user.id == r.user.id);
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  @override
  Widget build(BuildContext context) {
    return RefreshIndicator(
      onRefresh: _list.refresh,
      child: CursorListView<JoinRequest>(
        controller: _list,
        fetch: (cursor) => ref.read(communityApiProvider).joinRequests(widget.community.id, cursor: cursor),
        emptyText: widget.community.joinPolicy == JoinPolicy.approval
            ? 'No pending requests.'
            : 'Join requests appear here when "Who can join" needs approval.',
        itemBuilder: (context, r, _) => UserTile(
          user: r.user,
          subtitle: 'Requested ${timeAgo(r.requestedAt)}',
          onTap: () => context.push(Routes.profile(r.user.id)),
          trailing: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              IconButton(tooltip: 'Reject', icon: const Icon(Icons.close), onPressed: () => _decide(r, false)),
              IconButton(tooltip: 'Approve', icon: const Icon(Icons.check), onPressed: () => _decide(r, true)),
            ],
          ),
        ),
      ),
    );
  }
}

class _InviteTab extends ConsumerStatefulWidget {
  const _InviteTab({required this.community});

  final Community community;

  @override
  ConsumerState<_InviteTab> createState() => _InviteTabState();
}

class _InviteTabState extends ConsumerState<_InviteTab> {
  final _q = TextEditingController();
  final Map<int, UserSummary> _selected = {};
  String _query = '';
  Timer? _debounce;
  bool _sending = false;

  @override
  void dispose() {
    _q.dispose();
    _debounce?.cancel();
    super.dispose();
  }

  void _onQuery(String v) {
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 400), () {
      if (mounted) setState(() => _query = v.trim());
    });
  }

  Future<void> _send() async {
    if (_selected.isEmpty || _sending) return;
    setState(() => _sending = true);
    try {
      final res = await ref.read(communityControllerProvider).invite(widget.community.id, _selected.keys.toList());
      if (!mounted) return;
      final skippedNames = res.skipped.map((id) => _selected[id]?.displayName ?? 'Someone').toList();
      final text = StringBuffer('Invited ${res.invited.length}.');
      if (skippedNames.isNotEmpty) {
        text.write(' Not invited (already a member, already invited, or not allowed): ${skippedNames.join(', ')}.');
      }
      showInfoSnack(context, text.toString());
      setState(_selected.clear);
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final me = ref.watch(currentUserIdProvider);
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
          child: TextField(
            controller: _q,
            onChanged: _onQuery,
            decoration: const InputDecoration(prefixIcon: Icon(Icons.search), hintText: 'Search people to invite'),
          ),
        ),
        if (_selected.isNotEmpty)
          SizedBox(
            height: 48,
            child: ListView(
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.symmetric(horizontal: 12),
              children: [
                for (final u in _selected.values)
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 4),
                    child: InputChip(
                      label: Text(u.displayName),
                      onDeleted: () => setState(() => _selected.remove(u.id)),
                    ),
                  ),
              ],
            ),
          ),
        Expanded(
          child: _query.isEmpty
              ? const EmptyView(text: 'Search by name to find people.', icon: Icons.person_search_outlined)
              : CursorListView<UserSummary>(
                  key: ValueKey(_query),
                  fetch: (cursor) => ref.read(communityApiProvider).searchPeople(_query, cursor: cursor),
                  emptyText: 'No people found.',
                  itemBuilder: (context, u, _) => u.id == me
                      ? const SizedBox.shrink()
                      : UserTile(
                          user: u,
                          onTap: () => setState(() {
                            if (_selected.remove(u.id) == null) _selected[u.id] = u;
                          }),
                          trailing: Checkbox(
                            value: _selected.containsKey(u.id),
                            onChanged: (v) => setState(() {
                              if (v ?? false) {
                                _selected[u.id] = u;
                              } else {
                                _selected.remove(u.id);
                              }
                            }),
                          ),
                        ),
                ),
        ),
        SafeArea(
          top: false,
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: SizedBox(
              width: double.infinity,
              child: FilledButton.icon(
                onPressed: _selected.isEmpty || _sending ? null : _send,
                icon: _sending
                    ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
                    : const Icon(Icons.send_outlined),
                label: Text(_selected.isEmpty ? 'Invite' : 'Invite ${_selected.length}'),
              ),
            ),
          ),
        ),
      ],
    );
  }
}

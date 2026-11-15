import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/user_tile.dart';
import '../data/chat_api.dart';
import '../data/chat_models.dart';

/// Read-only member list of a conversation. Community moderation lives in
/// `communities/presentation/community_manage_screen.dart`.
class ChatMembersScreen extends ConsumerWidget {
  const ChatMembersScreen({super.key, required this.conversationId});

  final int conversationId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Scaffold(
      appBar: AppBar(title: const Text('Members')),
      body: CursorListView<ConversationMember>(
        fetch: (cursor) => ref.read(chatApiProvider).members(conversationId, cursor: cursor),
        emptyText: 'No members.',
        itemBuilder: (context, m, _) => UserTile(
          user: m.user,
          subtitle: _subtitle(m),
          onTap: () => context.push(Routes.profile(m.user.id)),
        ),
      ),
    );
  }

  static String? _subtitle(ConversationMember m) {
    final parts = [
      if (m.role != MemberRole.member) humanize(m.role),
      if (m.joinedAt != null) 'Joined ${formatDate(m.joinedAt)}',
    ];
    return parts.isEmpty ? null : parts.join(' · ');
  }
}

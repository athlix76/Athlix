import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/format/dates.dart';
import '../../../core/widgets/cursor_list_view.dart';
import '../../../core/widgets/user_tile.dart';
import '../data/profile_api.dart';
import '../data/profile_models.dart';

/// Who viewed my profile. Needs benefit `see_profile_viewers`; without it the
/// server returns 403 `PLAN_REQUIRED` and the list shows the plan-required view.
class ProfileViewersScreen extends ConsumerWidget {
  const ProfileViewersScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final api = ref.watch(profileApiProvider);
    final days = ref.watch(appConfigProvider).profileViewersWindowDays;
    return Scaffold(
      appBar: AppBar(title: const Text('Profile viewers')),
      body: CursorListView<ProfileViewer>(
        fetch: (cursor) => api.viewers(cursor: cursor),
        emptyText: 'No one has viewed your profile recently.',
        separated: true,
        header: days == null
            ? null
            : Padding(
                padding: const EdgeInsets.fromLTRB(16, 12, 16, 4),
                child: Text('Views from the last $days days.', style: Theme.of(context).textTheme.bodySmall),
              ),
        itemBuilder: (context, v, _) => UserTile(
          user: v.user,
          subtitle: [
            if (v.user.subtitle.isNotEmpty) v.user.subtitle,
            if (v.viewedOn != null) 'Viewed ${formatDate(v.viewedOn)}',
          ].join('\n'),
        ),
      ),
    );
  }
}

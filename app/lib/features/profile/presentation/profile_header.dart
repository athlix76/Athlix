import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/router/routes.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/widgets/user_avatar.dart';
import '../data/profile_models.dart';

/// Cover band, avatar, name, badges and follower counts, laid out like the UI template's
/// profile card. Shared by the profile and Me screens.
class ProfileHeader extends StatelessWidget {
  const ProfileHeader({super.key, required this.profile, this.actions = const []});

  final Profile profile;
  final List<Widget> actions;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final u = profile.user;
    final place = [u.city, u.state].whereType<String>().where((s) => s.isNotEmpty).join(', ');
    final headline = [u.sport?.name, u.position?.name].whereType<String>().join(' · ');
    return Container(
      color: AppTheme.surface,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // Cover band with the avatar overlapping its lower edge.
          SizedBox(
            height: 186,
            child: Stack(
              clipBehavior: Clip.none,
              children: [
                const Positioned.fill(bottom: 52, child: ClipRect(child: _Cover())),
                Positioned(
                  left: 20,
                  bottom: 0,
                  child: Container(
                    padding: const EdgeInsets.all(4),
                    decoration: const BoxDecoration(color: AppTheme.surface, shape: BoxShape.circle),
                    child: UserAvatar(name: u.displayName, url: u.photoUrl, radius: 46),
                  ),
                ),
                if (u.primaryRole != null)
                  Positioned(
                    right: 20,
                    bottom: 8,
                    child: DecoratedBox(
                      decoration: BoxDecoration(color: AppTheme.avatarBg, borderRadius: BorderRadius.circular(20)),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 5),
                        child: Text(
                          humanize(u.primaryRole!),
                          style: const TextStyle(color: AppTheme.avatarFg, fontWeight: FontWeight.w800, fontSize: 12),
                        ),
                      ),
                    ),
                  ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 14, 20, 16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(u.displayName, style: theme.textTheme.headlineSmall, overflow: TextOverflow.ellipsis),
                    ),
                    if (u.isVerified) const Icon(Icons.verified_outlined, size: 22, color: AppTheme.avatarFg),
                  ],
                ),
                if (headline.isNotEmpty) ...[
                  const SizedBox(height: 4),
                  Text(headline, style: theme.textTheme.bodyMedium?.copyWith(fontWeight: FontWeight.w700)),
                ],
                if (place.isNotEmpty) ...[
                  const SizedBox(height: 6),
                  Row(
                    children: [
                      const Icon(Icons.place_outlined, size: 16, color: AppTheme.slate),
                      const SizedBox(width: 4),
                      Expanded(child: Text(place, style: theme.textTheme.bodySmall)),
                    ],
                  ),
                ],
                if (profile.badges.isNotEmpty) ...[
                  const SizedBox(height: 10),
                  Wrap(
                    spacing: 6,
                    runSpacing: 6,
                    children: [
                      for (final b in profile.badges)
                        Chip(
                          avatar: const Icon(Icons.workspace_premium, size: 16, color: AppTheme.avatarFg),
                          label: Text(humanize(b)),
                          visualDensity: VisualDensity.compact,
                        ),
                    ],
                  ),
                ],
                if (profile.isFollowedBy && !profile.isOwn) ...[
                  const SizedBox(height: 8),
                  const Chip(label: Text('Follows you'), visualDensity: VisualDensity.compact),
                ],
                const SizedBox(height: 16),
                // Counts in three equal columns with thin dividers.
                IntrinsicHeight(
                  child: Row(
                    children: [
                      Expanded(
                        child: _Count(
                          label: 'Followers',
                          value: profile.followersCount,
                          onTap: () => context.push(Routes.followers(u.id)),
                        ),
                      ),
                      const VerticalDivider(width: 1),
                      Expanded(
                        child: _Count(
                          label: 'Following',
                          value: profile.followingCount,
                          onTap: () => context.push(Routes.following(u.id)),
                        ),
                      ),
                      const VerticalDivider(width: 1),
                      Expanded(child: _Count(label: 'Sports', value: profile.sports.length)),
                    ],
                  ),
                ),
                if (actions.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  actions.length <= 2
                      ? Row(
                          children: [
                            for (var i = 0; i < actions.length; i++) ...[
                              if (i > 0) const SizedBox(width: 10),
                              Expanded(child: actions[i]),
                            ],
                          ],
                        )
                      : Wrap(spacing: 8, runSpacing: 8, children: actions),
                ],
              ],
            ),
          ),
          const Divider(),
        ],
      ),
    );
  }
}

/// Navy cover with the template's slogan and soft track lines.
class _Cover extends StatelessWidget {
  const _Cover();

  @override
  Widget build(BuildContext context) => DecoratedBox(
        decoration: const BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: [Color(0xFF0B2233), Color(0xFF0E4A5C)],
          ),
        ),
        child: CustomPaint(
          painter: _TrackLines(),
          child: const Padding(
            padding: EdgeInsets.fromLTRB(22, 22, 18, 12),
            child: Stack(
              children: [
                Text(
                  'ALWAYS IN\nTHE MAKING.',
                  style: TextStyle(
                    fontFamily: AppTheme.headingFont,
                    color: AppTheme.white,
                    fontWeight: FontWeight.w800,
                    fontSize: 18,
                    height: 1.35,
                    letterSpacing: 1.2,
                  ),
                ),
                Align(
                  alignment: Alignment.bottomRight,
                  child: Text(
                    'ATHLIX COMMUNITY',
                    style: TextStyle(color: AppTheme.heroMuted, fontSize: 9, fontWeight: FontWeight.w700, letterSpacing: 1.2),
                  ),
                ),
              ],
            ),
          ),
        ),
      );
}

class _TrackLines extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.2
      ..color = AppTheme.cyan.withValues(alpha: 0.22);
    final c = Offset(size.width * 0.95, size.height * 1.05);
    for (var r = 70.0; r < size.width * 0.8; r += 26) {
      canvas.drawCircle(c, r, paint);
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

class _Count extends StatelessWidget {
  const _Count({required this.label, required this.value, this.onTap});

  final String label;
  final int value;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(8),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 4),
        child: Column(
          children: [
            Text(compactCount(value), style: theme.textTheme.titleLarge),
            const SizedBox(height: 2),
            Text(label, style: theme.textTheme.bodySmall),
          ],
        ),
      ),
    );
  }
}

/// 1234 -> 1.2K, 1500000 -> 1.5M (as on the template's profile).
String compactCount(int n) {
  if (n < 1000) return '$n';
  String one(double v) => v >= 100 ? v.toStringAsFixed(0) : v.toStringAsFixed(1).replaceAll(RegExp(r'\.0$'), '');
  if (n < 1000000) return '${one(n / 1000)}K';
  return '${one(n / 1000000)}M';
}

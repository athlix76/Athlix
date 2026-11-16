import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:intl/intl.dart';

import '../../../core/format/dates.dart';
import '../../../core/money/money.dart';
import '../../../core/router/routes.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/widgets/user_avatar.dart';
import '../../location_identity/application/eligibility_view.dart';
import '../../location_identity/presentation/eligibility_chip.dart';
import '../data/event_models.dart';

/// Card for an [EventSummary] in lists. Events the caller is not eligible
/// for (only listed with `showAll=true`) are greyed with a "Verify now" button.
class EventCard extends StatelessWidget {
  const EventCard({super.key, required this.event, this.showStatus = false});

  final EventSummary event;
  final bool showStatus;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final banner = event.bannerUrl;
    final elig = EligibilityView.from(event.myEligibility);
    final distance = event.distanceKm;

    final start = event.startsAt?.toLocal();
    final body = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Banner (or a navy panel with the sport name) with the date badge.
        SizedBox(
          height: 140,
          child: Stack(
            fit: StackFit.expand,
            children: [
              if (banner != null)
                SignedImage(url: banner, height: 140)
              else
                DecoratedBox(
                  decoration: const BoxDecoration(
                    gradient: LinearGradient(
                      begin: Alignment.topLeft,
                      end: Alignment.bottomRight,
                      colors: [AppTheme.navy, Color(0xFF0B3A4A)],
                    ),
                  ),
                  child: Align(
                    alignment: Alignment.bottomLeft,
                    child: Padding(
                      padding: const EdgeInsets.fromLTRB(18, 0, 18, 18),
                      child: Text(
                        event.sport?.name ?? humanize(event.eventType),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontFamily: AppTheme.headingFont,
                          color: AppTheme.white,
                          fontSize: 32,
                          fontWeight: FontWeight.w800,
                          letterSpacing: -1,
                        ),
                      ),
                    ),
                  ),
                ),
              if (start != null)
                Positioned(
                  left: 14,
                  top: 14,
                  child: DecoratedBox(
                    decoration: BoxDecoration(color: AppTheme.white, borderRadius: BorderRadius.circular(10)),
                    child: Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                      child: Text.rich(
                        TextSpan(children: [
                          TextSpan(
                            text: '${start.day} ',
                            style: const TextStyle(fontFamily: AppTheme.headingFont, fontWeight: FontWeight.w800, fontSize: 18),
                          ),
                          TextSpan(
                            text: DateFormat('MMM').format(start).toUpperCase(),
                            style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 11),
                          ),
                        ]),
                        style: const TextStyle(color: AppTheme.navy),
                      ),
                    ),
                  ),
                ),
              if (event.isFeatured)
                Positioned(
                  right: 14,
                  top: 14,
                  child: DecoratedBox(
                    decoration: BoxDecoration(
                      color: AppTheme.navy.withValues(alpha: 0.6),
                      borderRadius: BorderRadius.circular(20),
                      border: Border.all(color: AppTheme.white.withValues(alpha: 0.3)),
                    ),
                    child: const Padding(
                      padding: EdgeInsets.symmetric(horizontal: 10, vertical: 4),
                      child: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Icon(Icons.star, size: 14, color: AppTheme.cyan),
                          SizedBox(width: 4),
                          Text('Featured', style: TextStyle(color: AppTheme.white, fontSize: 11, fontWeight: FontWeight.w700)),
                        ],
                      ),
                    ),
                  ),
                ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(18, 16, 18, 14),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                [event.sport?.name, humanize(event.eventType), humanize(event.competitionLevel)]
                    .whereType<String>()
                    .where((x) => x.isNotEmpty)
                    .join(' · ')
                    .toUpperCase(),
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  color: AppTheme.link,
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 1,
                ),
              ),
              const SizedBox(height: 6),
              Text(event.title, style: theme.textTheme.titleLarge),
              const SizedBox(height: 10),
              if (event.place.isNotEmpty || distance != null)
                _IconLine(
                  icon: Icons.place_outlined,
                  text: [
                    if (event.place.isNotEmpty) event.place,
                    if (distance != null) '${distance.toStringAsFixed(1)} km away',
                  ].join(' · '),
                ),
              _IconLine(icon: Icons.schedule, text: formatDateTime(event.startsAt)),
              const Padding(padding: EdgeInsets.symmetric(vertical: 12), child: Divider()),
              Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          formatEntryFee(event.entryFeePaise) == 'Free' ? 'Free entry' : formatEntryFee(event.entryFeePaise),
                          style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w800),
                        ),
                        Text('${event.slotsLeft} of ${event.maxSlots} spots left', style: theme.textTheme.bodySmall),
                      ],
                    ),
                  ),
                  if (showStatus) ...[Chip(label: Text(humanize(event.status))), const SizedBox(width: 6)],
                  EligibilityChip(view: elig),
                ],
              ),
            ],
          ),
        ),
      ],
    );

    return Card(
      clipBehavior: Clip.antiAlias,
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
      child: InkWell(
        onTap: () => context.push(Routes.event(event.id)),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            elig.greyed ? Opacity(opacity: 0.5, child: body) : body,
            if (elig.greyed)
              Padding(
                padding: const EdgeInsets.fromLTRB(12, 0, 12, 12),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        elig.reasonText ?? '',
                        style: theme.textTheme.bodySmall,
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    if (elig.ctaLabel != null && elig.ctaRoute != null)
                      FilledButton.tonal(
                        onPressed: () => context.push(elig.ctaRoute!),
                        child: Text(elig.ctaLabel!),
                      ),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }
}

class _IconLine extends StatelessWidget {
  const _IconLine({required this.icon, required this.text});

  final IconData icon;
  final String text;

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.only(bottom: 4),
        child: Row(
          children: [
            Icon(icon, size: 17, color: AppTheme.slate),
            const SizedBox(width: 8),
            Expanded(child: Text(text, style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: AppTheme.slate))),
          ],
        ),
      );
}

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/money/money.dart';
import '../../../core/router/routes.dart';
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

    final body = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (banner != null) SignedImage(url: banner, height: 120),
        Padding(
          padding: const EdgeInsets.all(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(child: Text(event.title, style: theme.textTheme.titleMedium)),
                  if (event.isFeatured) const Icon(Icons.star, size: 18),
                ],
              ),
              const SizedBox(height: 4),
              Text(
                [event.sport?.name, humanize(event.eventType), humanize(event.competitionLevel)]
                    .whereType<String>()
                    .join(' · '),
                style: theme.textTheme.bodySmall,
              ),
              const SizedBox(height: 4),
              Text(formatDateTime(event.startsAt)),
              if (event.place.isNotEmpty || distance != null)
                Text(
                  [
                    if (event.place.isNotEmpty) event.place,
                    if (distance != null) '${distance.toStringAsFixed(1)} km away',
                  ].join(' · '),
                  style: theme.textTheme.bodySmall,
                ),
              const SizedBox(height: 8),
              Wrap(
                spacing: 8,
                runSpacing: 4,
                children: [
                  Chip(label: Text(formatEntryFee(event.entryFeePaise))),
                  Chip(label: Text('${event.slotsLeft} of ${event.maxSlots} slots left')),
                  if (showStatus) Chip(label: Text(humanize(event.status))),
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

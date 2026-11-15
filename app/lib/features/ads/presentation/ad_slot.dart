import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../core/widgets/user_avatar.dart';
import '../application/ad_providers.dart';
import '../data/ad_api.dart';
import '../data/ad_models.dart';

/// Placement codes accepted by `GET /ads?placement=`.
abstract final class AdPlacement {
  static const homeFeed = 'home_feed';
  static const eventsList = 'events_list';
  static const search = 'search';
  static const eventDetail = 'event_detail';
}

/// One sponsored card for [placement]. Renders nothing when there is no ad.
/// Records one impression per ad shown and a click on tap.
class AdSlot extends ConsumerStatefulWidget {
  const AdSlot({super.key, required this.placement, this.sportId, this.state});

  final String placement;
  final int? sportId;
  final String? state;

  @override
  ConsumerState<AdSlot> createState() => _AdSlotState();
}

class _AdSlotState extends ConsumerState<AdSlot> {
  final Set<int> _impressions = <int>{};

  AdQuery get _query => (placement: widget.placement, sportId: widget.sportId, state: widget.state);

  void _recordImpression(Ad ad) {
    if (!_impressions.add(ad.id)) return;
    // Fire and forget: tracking failures are not shown to the user.
    ref.read(adApiProvider).impression(ad.id).catchError((Object _) {});
  }

  Future<void> _open(Ad ad) async {
    ref.read(adApiProvider).click(ad.id).catchError((Object _) {});
    final uri = Uri.tryParse(ad.targetUrl);
    if (uri == null || !(uri.scheme == 'https' || uri.scheme == 'http')) return;
    await launchUrl(uri, mode: LaunchMode.externalApplication);
  }

  @override
  Widget build(BuildContext context) {
    final ads = ref.watch(adsProvider(_query)).valueOrNull ?? const <Ad>[];
    if (ads.isEmpty) return const SizedBox.shrink();
    final ad = ads.first;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _recordImpression(ad);
    });
    final theme = Theme.of(context);
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: () => _open(ad),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (ad.imageUrl != null) SignedImage(url: ad.imageUrl!, height: 140, fit: BoxFit.cover),
            Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('${ad.label} · ${ad.advertiserName}', style: theme.textTheme.labelSmall),
                  const SizedBox(height: 4),
                  Text(ad.title, style: theme.textTheme.titleSmall),
                  if (ad.body != null && ad.body!.isNotEmpty) ...[
                    const SizedBox(height: 4),
                    Text(ad.body!, maxLines: 3, overflow: TextOverflow.ellipsis),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

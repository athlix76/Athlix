import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_error.dart';
import '../../../core/format/format.dart';
import '../../../core/media/picked_file.dart';
import '../../../core/widgets/api_error_view.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../../../core/widgets/cursor_list.dart';
import '../../../core/widgets/date_time_field.dart';
import '../../../core/widgets/editor_dialog.dart';
import '../../sports/application/sports_providers.dart';
import '../../sports/data/sports_api.dart';
import '../application/ads_providers.dart';
import '../data/ads_api.dart';

/// Pre-check only; the server enforces `media.allowed_image_types` and the size limit.
const _adImageTypes = ['image/jpeg', 'image/png', 'image/webp'];

/// Ad campaigns: list, create, edit, pause/resume and daily stats.
/// Images are not uploaded from here (see README).
class AdsScreen extends ConsumerStatefulWidget {
  const AdsScreen({super.key});

  @override
  ConsumerState<AdsScreen> createState() => _AdsScreenState();
}

class _AdsScreenState extends ConsumerState<AdsScreen> {
  final _listKey = GlobalKey<CursorListState<AdCampaign>>();

  void _reload() => _listKey.currentState?.reload();

  Future<void> _edit(AdCampaign? existing) async {
    final api = ref.read(adsApiProvider);
    final sports = ref.read(adminSportsProvider).valueOrNull ?? const <AdminSport>[];
    // Controllers are not disposed: the dialog's exit animation may still use them.
    final advertiser = TextEditingController(text: existing?.advertiserName ?? '');
    final title = TextEditingController(text: existing?.title ?? '');
    final body = TextEditingController(text: existing?.body ?? '');
    final targetUrl = TextEditingController(text: existing?.targetUrl ?? 'https://');
    final state = TextEditingController(text: existing?.state ?? '');
    final priority = TextEditingController(text: '${existing?.priority ?? 0}');
    var placement = existing?.placement ?? adPlacements.first;
    var status = existing?.status ?? 'draft';
    int? sportId = existing?.sportId;
    DateTime? startsAt = existing?.startsAt;
    DateTime? endsAt = existing?.endsAt;
    int? imageUploadId = existing?.imageUploadId;
    var imageNote = existing?.imageUploadId == null ? 'No image' : 'Saved image #${existing!.imageUploadId}';
    var uploading = false;

    Future<void> pickImage(StateSetter setState) async {
      final f = await pickFile(accept: _adImageTypes.join(','));
      if (f == null) return;
      if (!_adImageTypes.contains(f.contentType)) {
        setState(() => imageNote = 'Use a JPEG, PNG or WebP image.');
        return;
      }
      setState(() {
        uploading = true;
        imageNote = 'Uploading ${f.name}…';
      });
      try {
        final id = await api.uploadImage(contentType: f.contentType, bytes: f.bytes);
        setState(() {
          imageUploadId = id;
          imageNote = '${f.name} uploaded (#$id)';
        });
      } on ApiError catch (e) {
        setState(() => imageNote = e.fields['sizeBytes'] ?? e.fields['contentType'] ?? e.message);
      } finally {
        setState(() => uploading = false);
      }
    }

    AdCampaignInput input() => AdCampaignInput(
          advertiserName: advertiser.text.trim(),
          title: title.text.trim(),
          body: body.text.trim().isEmpty ? null : body.text.trim(),
          imageUploadId: imageUploadId,
          targetUrl: targetUrl.text.trim(),
          placement: placement,
          sportId: sportId,
          state: state.text.trim().isEmpty ? null : state.text.trim(),
          priority: int.tryParse(priority.text.trim()) ?? 0,
          startsAt: startsAt!,
          endsAt: endsAt!,
          status: status,
        );

    final saved = await showEditorDialog(
      context,
      title: existing == null ? 'New ad campaign' : 'Edit ad #${existing.id}',
      width: 620,
      builder: (ctx, setState) => formColumn([
        FormText(label: 'Advertiser name', controller: advertiser),
        FormText(label: 'Title', controller: title),
        FormText(label: 'Body (optional)', controller: body, maxLines: 3),
        Row(children: [
          Expanded(child: Text('Image: $imageNote')),
          if (imageUploadId != null && !uploading)
            TextButton(onPressed: () => setState(() {
                  imageUploadId = null;
                  imageNote = 'No image';
                }), child: const Text('Remove')),
          OutlinedButton.icon(
            onPressed: uploading ? null : () => pickImage(setState),
            icon: const Icon(Icons.image_outlined),
            label: Text(imageUploadId == null ? 'Choose image…' : 'Replace…'),
          ),
        ]),
        FormText(label: 'Target URL (https only)', controller: targetUrl, keyboardType: TextInputType.url),
        FormDropdown<String>(
          label: 'Placement',
          value: placement,
          items: [for (final p in {...adPlacements, placement}) (p, p.replaceAll('_', ' '))],
          onChanged: (v) => setState(() => placement = v ?? placement),
        ),
        FormDropdown<int>(
          label: 'Sport',
          value: sportId,
          nullLabel: 'All sports',
          items: [
            for (final s in sports) (s.id, s.name),
            // Keep the saved sport selectable while the list is loading or not readable.
            if (sportId != null && !sports.any((s) => s.id == sportId)) (sportId!, 'Sport #$sportId'),
          ],
          onChanged: (v) => setState(() => sportId = v),
        ),
        FormText(label: 'State (optional, blank = all India)', controller: state),
        FormText(label: 'Priority (higher shows first)', controller: priority, keyboardType: TextInputType.number),
        DateTimeField(label: 'Starts at', value: startsAt, onChanged: (v) => setState(() => startsAt = v)),
        DateTimeField(label: 'Ends at', value: endsAt, onChanged: (v) => setState(() => endsAt = v)),
        FormDropdown<String>(
          label: 'Status',
          value: status,
          items: [for (final s in {...adStatuses, status}) (s, s)],
          onChanged: (v) => setState(() => status = v ?? status),
        ),
      ]),
      validate: () {
        final e = <String, String>{};
        if (uploading) e['imageUploadId'] = 'Wait for the image upload to finish';
        if (advertiser.text.trim().isEmpty) e['advertiserName'] = 'Required';
        if (title.text.trim().isEmpty) e['title'] = 'Required';
        final uri = Uri.tryParse(targetUrl.text.trim());
        if (uri == null || uri.scheme != 'https' || uri.host.isEmpty) e['targetUrl'] = 'Must be an https URL';
        if (int.tryParse(priority.text.trim()) == null) e['priority'] = 'Whole number';
        if (startsAt == null) e['startsAt'] = 'Required';
        if (endsAt == null) e['endsAt'] = 'Required';
        if (startsAt != null && endsAt != null && !endsAt!.isAfter(startsAt!)) e['endsAt'] = 'Must be after the start';
        return e;
      },
      confirmMessage: existing == null
          ? 'Create this campaign? It is shown to users only while active and inside its dates.'
          : 'Save changes to this campaign? Live changes apply to the next ad request.',
      save: () => existing == null ? api.create(input()) : api.update(existing.id, input()),
    );
    if (saved) _reload();
  }

  Future<void> _setStatus(AdCampaign a, String status) async {
    final r = await confirmAction<AdCampaign>(
      context,
      title: '${status == 'paused' ? 'Pause' : status == 'ended' ? 'End' : 'Activate'} "${a.title}"?',
      message: status == 'ended' ? 'Ending stops the campaign for good. To run it later, pause it instead.' : 'Applies to the next ad request.',
      confirmLabel: 'Confirm',
      destructive: status == 'ended',
      run: (_) => ref.read(adsApiProvider).setStatus(a.id, status),
      successMessage: 'Status changed',
    );
    if (r != null) _reload();
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(adsApiProvider);
    ref.watch(adminSportsProvider); // warm the sport list for the editor
    return PageScaffold(
      title: 'Ads',
      subtitle: 'Ads never show to minors unless ads.show_to_minors is on. Image upload is not available here yet.',
      actions: [FilledButton.icon(onPressed: () => _edit(null), icon: const Icon(Icons.add), label: const Text('New campaign'))],
      body: CursorList<AdCampaign>(
        key: _listKey,
        load: (cursor) => api.list(cursor: cursor),
        emptyText: 'No campaigns.',
        itemBuilder: (context, a) => ListTile(
          title: Text('${a.title}  ·  ${a.advertiserName}'),
          subtitle: Text([
            a.placement.replaceAll('_', ' '),
            'priority ${a.priority}',
            '${formatDateTime(a.startsAt)} to ${formatDateTime(a.endsAt)}',
            if (a.state != null) a.state!,
            a.targetUrl,
          ].join('  ·  ')),
          trailing: Wrap(spacing: 4, crossAxisAlignment: WrapCrossAlignment.center, children: [
            StatusChip(a.status),
            IconButton(tooltip: 'Stats', icon: const Icon(Icons.bar_chart), onPressed: () => showAdStats(context, a)),
            IconButton(tooltip: 'Edit', icon: const Icon(Icons.edit), onPressed: () => _edit(a)),
            PopupMenuButton<String>(
              tooltip: 'Status',
              onSelected: (s) => _setStatus(a, s),
              itemBuilder: (_) => [
                if (a.status != 'active' && a.status != 'ended') const PopupMenuItem(value: 'active', child: Text('Activate')),
                if (a.status == 'active') const PopupMenuItem(value: 'paused', child: Text('Pause')),
                if (a.status != 'ended') const PopupMenuItem(value: 'ended', child: Text('End')),
              ],
            ),
          ]),
        ),
      ),
    );
  }
}

String _ymd(DateTime d) => '${d.year.toString().padLeft(4, '0')}-${d.month.toString().padLeft(2, '0')}-${d.day.toString().padLeft(2, '0')}';

/// Daily impressions and clicks for a date range (default: the last 30 days).
Future<void> showAdStats(BuildContext context, AdCampaign ad) =>
    showDialog<void>(context: context, builder: (_) => _AdStatsDialog(ad: ad));

class _AdStatsDialog extends ConsumerStatefulWidget {
  const _AdStatsDialog({required this.ad});

  final AdCampaign ad;

  @override
  ConsumerState<_AdStatsDialog> createState() => _AdStatsDialogState();
}

class _AdStatsDialogState extends ConsumerState<_AdStatsDialog> {
  late DateTime _from = DateTime.now().subtract(const Duration(days: 30));
  late DateTime _to = DateTime.now();
  Future<List<AdDayStats>>? _future;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _load() {
    if (_to.isBefore(_from)) {
      _future = Future.error(ApiError.invalidInput({'to': 'Must be on or after the start date'}));
    } else {
      _future = ref.read(adsApiProvider).stats(widget.ad.id, from: _ymd(_from.toLocal()), to: _ymd(_to.toLocal()));
    }
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text('Stats: ${widget.ad.title}'),
      content: SizedBox(
        width: 520,
        height: 420,
        child: Column(children: [
          Row(children: [
            Expanded(child: DateTimeField(label: 'From', dateOnly: true, value: _from, onChanged: (v) => setState(() { if (v != null) _from = v; _load(); }))),
            const SizedBox(width: 12),
            Expanded(child: DateTimeField(label: 'To', dateOnly: true, value: _to, onChanged: (v) => setState(() { if (v != null) _to = v; _load(); }))),
          ]),
          const SizedBox(height: 12),
          Expanded(
            child: FutureBuilder<List<AdDayStats>>(
              future: _future,
              builder: (context, snap) {
                if (snap.hasError) return ApiErrorView(error: snap.error!, onRetry: () => setState(_load));
                if (!snap.hasData) return const Center(child: CircularProgressIndicator());
                final rows = snap.data!;
                if (rows.isEmpty) return const Center(child: Text('No data for this range.'));
                final imp = rows.fold<int>(0, (s, r) => s + r.impressions);
                final clk = rows.fold<int>(0, (s, r) => s + r.clicks);
                final ctr = imp == 0 ? '—' : '${(clk * 10000 ~/ imp) / 100}%';
                return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text('Total: $imp impressions, $clk clicks, CTR $ctr'),
                  const SizedBox(height: 8),
                  Expanded(
                    child: SingleChildScrollView(
                      child: DataTable(columns: const [
                        DataColumn(label: Text('Date')),
                        DataColumn(label: Text('Impressions'), numeric: true),
                        DataColumn(label: Text('Clicks'), numeric: true),
                      ], rows: [
                        for (final r in rows)
                          DataRow(cells: [DataCell(Text(r.date)), DataCell(Text('${r.impressions}')), DataCell(Text('${r.clicks}'))]),
                      ]),
                    ),
                  ),
                ]);
              },
            ),
          ),
        ]),
      ),
      actions: [TextButton(onPressed: () => Navigator.of(context).pop(), child: const Text('Close'))],
    );
  }
}

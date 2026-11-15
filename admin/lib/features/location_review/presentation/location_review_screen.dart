import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/auth/permissions.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/cursor_list.dart';
import '../../auth/application/session_controller.dart';
import '../../districts/application/districts_providers.dart';
import '../application/location_review_providers.dart';
import '../data/location_review_api.dart';

/// Queue of native / residence identities. Pending rows come oldest first
/// (the backend orders them).
class LocationReviewScreen extends ConsumerStatefulWidget {
  const LocationReviewScreen({super.key});

  @override
  ConsumerState<LocationReviewScreen> createState() => _LocationReviewScreenState();
}

class _LocationReviewScreenState extends ConsumerState<LocationReviewScreen> {
  final _listKey = GlobalKey<CursorListState<LocationRow>>();
  final _stateId = TextEditingController();
  final _districtId = TextEditingController();
  String? _status = 'pending';
  String? _type;
  int? _district;

  @override
  void dispose() {
    _stateId.dispose();
    _districtId.dispose();
    super.dispose();
  }

  int? get _stateIdValue => int.tryParse(_stateId.text.trim());

  LocationQueueFilters get _filters => LocationQueueFilters(
        status: _status,
        identityType: _type,
        stateId: _stateIdValue,
        districtId: _district ?? int.tryParse(_districtId.text.trim()),
      );

  void _reload() => _listKey.currentState?.reload();

  Widget _districtFilter(bool canReadDistricts) {
    final stateId = _stateIdValue;
    // The district list is a config.read endpoint; reviewers without it type an id.
    if (!canReadDistricts || stateId == null) {
      return FilterTextField(
        label: 'District id',
        controller: _districtId,
        width: 160,
        onSubmitted: (_) => setState(_reload),
      );
    }
    final districts = ref.watch(districtsOfStateProvider(stateId));
    final items = districts.valueOrNull ?? const [];
    return SizedBox(
      width: 220,
      child: DropdownButtonFormField<int?>(
        value: items.any((d) => d.id == _district) ? _district : null,
        isExpanded: true,
        decoration: const InputDecoration(labelText: 'District', border: OutlineInputBorder(), isDense: true),
        items: [
          const DropdownMenuItem<int?>(value: null, child: Text('Any')),
          for (final d in items) DropdownMenuItem<int?>(value: d.id, child: Text(d.name)),
        ],
        onChanged: (v) {
          setState(() => _district = v);
          _reload();
        },
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final api = ref.watch(locationReviewApiProvider);
    final canReadDistricts = ref.watch(sessionProvider).can(Perm.configRead);
    return PageScaffold(
      title: 'Location review',
      subtitle: 'Native and residence proofs. Check the document against the checklist, then approve or reject.',
      filters: Wrap(spacing: 12, runSpacing: 12, children: [
        FilterDropdown(
          label: 'Status',
          value: _status,
          options: identityStatuses,
          onChanged: (v) {
            setState(() => _status = v);
            _reload();
          },
        ),
        FilterDropdown(
          label: 'Type',
          value: _type,
          options: identityTypes,
          onChanged: (v) {
            setState(() => _type = v);
            _reload();
          },
        ),
        FilterTextField(
          label: 'State id',
          controller: _stateId,
          width: 140,
          onSubmitted: (_) {
            setState(() {
              _district = null;
              _districtId.clear();
            });
            _reload();
          },
        ),
        _districtFilter(canReadDistricts),
      ]),
      body: CursorList<LocationRow>(
        key: _listKey,
        load: (cursor) => api.queue(_filters, cursor),
        emptyText: 'Nothing to review.',
        itemBuilder: (context, r) => ListTile(
          leading: Icon(r.identity.identityType == 'native' ? Icons.home_outlined : Icons.apartment_outlined),
          title: Text('${r.user.label}  ·  ${r.identity.identityType}  ·  ${r.identity.placeLabel}'),
          subtitle: Text([
            'proof ${r.identity.proofType}',
            if (r.identity.proofHolder != 'self') 'holder ${r.identity.proofHolder}',
            if (r.isMinor) 'minor',
            'submitted ${formatDateTime(r.identity.submittedAt)}',
          ].join('  ·  ')),
          trailing: StatusChip(r.identity.status),
          onTap: () => context.go('/location-review/${r.identity.id}'),
        ),
      ),
    );
  }
}

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_error.dart';
import '../../../core/format/format.dart';
import '../../../core/widgets/async_body.dart';
import '../../../core/widgets/common.dart';
import '../../../core/widgets/confirm_action.dart';
import '../application/location_review_providers.dart';
import '../data/location_review_api.dart';
import 'proof_viewer.dart';

String _label(String code) => code.replaceAll('_', ' ');

/// Rejection code whose rejection deletes the proof file at once.
const _unmaskedAadhaar = 'unmasked_aadhaar';

class LocationDetailScreen extends ConsumerWidget {
  const LocationDetailScreen({super.key, required this.identityId});

  final int identityId;

  void _refetch(WidgetRef ref) => ref.invalidate(locationDetailProvider(identityId));

  Future<void> _approve(BuildContext context, WidgetRef ref, LocationDetail d) async {
    final api = ref.read(locationReviewApiProvider);
    final masked = d.identity.proofType == maskedAadhaarProof;
    // Not disposed: the dialog's exit animation may still use it (see editor_dialog.dart).
    final last4 = TextEditingController();
    final r = await confirmAction<LocationDetail>(
      context,
      title: 'Approve ${d.identity.identityType} identity?',
      message: 'The user becomes ${d.identity.identityType} of ${d.identity.placeLabel}. '
          'An earlier verified ${d.identity.identityType} identity is superseded'
          '${d.identity.identityType == 'residence' ? ', and residence gets an expiry date from config' : ''}.',
      confirmLabel: 'Approve',
      details: masked
          ? TextField(
              controller: last4,
              maxLength: 4,
              keyboardType: TextInputType.number,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              decoration: const InputDecoration(
                labelText: 'Aadhaar last 4 digits (optional)',
                helperText: 'Only the 4 digits visible on the masked Aadhaar. Nothing else is stored.',
                border: OutlineInputBorder(),
              ),
            )
          : null,
      reason: const ReasonSpec(label: 'Internal note', required: false, hint: 'Admins only, at most 255 characters'),
      run: (note) {
        final digits = last4.text.trim();
        if (masked && digits.isNotEmpty && !RegExp(r'^\d{4}$').hasMatch(digits)) {
          throw ApiError.invalidInput({'aadhaarLast4': 'Enter exactly 4 digits, or leave it empty'});
        }
        return api.approve(identityId, aadhaarLast4: masked && digits.isNotEmpty ? digits : null, note: note);
      },
      successMessage: 'Identity approved',
    );
    if (r != null) _refetch(ref);
  }

  Future<void> _reject(BuildContext context, WidgetRef ref, LocationDetail d) async {
    final api = ref.read(locationReviewApiProvider);
    String? code;
    final r = await confirmAction<LocationDetail>(
      context,
      title: 'Reject identity?',
      message: 'The user sees the reason and can submit a new proof.',
      confirmLabel: 'Reject',
      destructive: true,
      details: StatefulBuilder(
        builder: (context, setState) => Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          DropdownButtonFormField<String>(
            value: code,
            isExpanded: true,
            decoration: const InputDecoration(labelText: 'Reason code', border: OutlineInputBorder()),
            items: [for (final c in d.rejectionReasons) DropdownMenuItem(value: c, child: Text(_label(c)))],
            onChanged: (v) => setState(() => code = v),
          ),
          if (code == _unmaskedAadhaar)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                'The proof file is deleted immediately because it shows a full Aadhaar number.',
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
        ]),
      ),
      reason: const ReasonSpec(label: 'Note to the user', required: false, hint: 'What to fix (shown to the user)'),
      run: (note) {
        final c = code;
        if (c == null) throw ApiError.invalidInput({'rejectionCode': 'Choose a reason code'});
        return api.reject(identityId, rejectionCode: c, rejectionReason: note);
      },
      successMessage: 'Identity rejected',
    );
    if (r != null) _refetch(ref);
  }

  Future<void> _revoke(BuildContext context, WidgetRef ref) async {
    final api = ref.read(locationReviewApiProvider);
    final r = await confirmAction<LocationDetail>(
      context,
      title: 'Revoke verified identity?',
      message: 'Use this for fraud or a mistaken approval. Registrations that relied on it are flagged for '
          'the organiser (never cancelled automatically).',
      confirmLabel: 'Revoke',
      destructive: true,
      reason: const ReasonSpec(hint: 'Why it is being revoked (audited)'),
      run: (reason) => api.revoke(identityId, reason!),
      successMessage: 'Identity revoked',
    );
    if (r != null) _refetch(ref);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(locationDetailProvider(identityId));
    return PageScaffold(
      title: 'Location identity #$identityId',
      actions: [IconButton(tooltip: 'Reload (audited proof view)', icon: const Icon(Icons.refresh), onPressed: () => _refetch(ref))],
      body: AsyncBody<LocationDetail>(
        value: value,
        onRetry: () => _refetch(ref),
        data: (d) {
          final i = d.identity;
          return SingleChildScrollView(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              SectionCard(
                title: '${_label(i.identityType)}: ${i.placeLabel}',
                actions: [
                  StatusChip(i.status),
                  if (i.status == 'pending') ...[
                    const SizedBox(width: 8),
                    OutlinedButton(onPressed: () => _reject(context, ref, d), child: const Text('Reject')),
                    const SizedBox(width: 8),
                    FilledButton(onPressed: () => _approve(context, ref, d), child: const Text('Approve')),
                  ],
                  if (i.status == 'verified') ...[
                    const SizedBox(width: 8),
                    OutlinedButton(onPressed: () => _revoke(context, ref), child: const Text('Revoke')),
                  ],
                ],
                child: KeyValueList([
                  ('User', textValue(d.row.user.label)),
                  ('Minor', textValue(d.row.isMinor ? 'yes (guardian proof allowed)' : 'no')),
                  ('Profile district (hint only)', textValue(d.profileDistrict)),
                  ('Source', textValue(_label(i.source))),
                  ('Proof type', textValue(_label(i.proofType))),
                  ('Proof holder', textValue(_label(i.proofHolder))),
                  ('Aadhaar last 4', textValue(i.aadhaarLast4)),
                  ('Submitted', textValue(formatDateTime(i.submittedAt))),
                  ('Reviewed', textValue(i.reviewedAt == null ? null : [formatDateTime(i.reviewedAt), if (d.reviewedBy != null) d.reviewedBy!.name].join(' by '))),
                  if (d.reviewNote != null) ('Internal note', textValue(d.reviewNote)),
                  ('Verified', textValue(formatDateTime(i.verifiedAt))),
                  ('Expires', textValue(formatDateTime(i.expiresAt))),
                  if (i.rejectionCode != null) ('Rejection', textValue([_label(i.rejectionCode!), i.rejectionReason].whereType<String>().join(': '))),
                ]),
              ),
              SectionCard(title: 'Checklist', child: _Checklist(d)),
              SectionCard(title: 'Proof', child: ProofViewer(detail: d, onRefetch: () => _refetch(ref))),
              SectionCard(
                title: 'Earlier ${_label(i.identityType)} identities of this user',
                child: d.history.isEmpty
                    ? const Text('None.')
                    : Column(children: [
                        for (final h in d.history)
                          ListTile(
                            dense: true,
                            contentPadding: EdgeInsets.zero,
                            title: Text('#${h.id}  ·  ${h.placeLabel}  ·  ${_label(h.proofType)}'),
                            subtitle: Text([
                              'submitted ${formatDateTime(h.submittedAt)}',
                              if (h.verifiedAt != null) 'verified ${formatDateTime(h.verifiedAt)}',
                              if (h.expiresAt != null) 'expires ${formatDateTime(h.expiresAt)}',
                              if (h.rejectionCode != null) 'rejected: ${_label(h.rejectionCode!)}',
                            ].join('  ·  ')),
                            trailing: StatusChip(h.status),
                          ),
                      ]),
              ),
            ]),
          );
        },
      ),
    );
  }
}

class _Checklist extends StatelessWidget {
  const _Checklist(this.d);

  final LocationDetail d;

  @override
  Widget build(BuildContext context) {
    final i = d.identity;
    final items = <String>[
      'Aadhaar must be masked: only the last 4 digits may be visible. If any document shows a full '
          'Aadhaar number, reject with "${_label(_unmaskedAadhaar)}" (the file is deleted at once).',
      'The address on the proof is in ${i.placeLabel.isEmpty ? 'the claimed district' : i.placeLabel}.',
      i.proofHolder == 'self'
          ? 'The name on the proof matches the user.'
          : 'The proof is in the ${_label(i.proofHolder)}\'s name${d.row.isMinor ? ' (allowed for a minor)' : ''}; check the relationship is plausible.',
      if (datedProofTypes.contains(i.proofType))
        d.utilityBillMaxAgeDays == null
            ? 'The ${_label(i.proofType)} is recent (see config for the maximum age).'
            : 'The ${_label(i.proofType)} is dated within the last ${d.utilityBillMaxAgeDays} days.',
      if (i.identityType == 'native') 'The proof shows birth or permanent origin in the district, not just a current address.',
      'Do not copy, download or forward the document.',
    ];
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      for (final t in items)
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 3),
          child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
            const Icon(Icons.check_box_outline_blank, size: 18),
            const SizedBox(width: 8),
            Expanded(child: Text(t)),
          ]),
        ),
    ]);
  }
}

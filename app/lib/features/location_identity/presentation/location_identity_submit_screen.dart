import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/format/dates.dart';
import '../../../core/media/media_upload.dart';
import '../../../core/media/pick_file.dart';
import '../../../core/models/policy.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/sport_picker.dart';
import '../application/location_identity_controller.dart';
import '../data/location_identity_models.dart';
import 'geo_pickers.dart';

const _proofExtensions = ['pdf', 'jpg', 'jpeg', 'png'];

const proofTypeLabels = {
  'digilocker_aadhaar': 'Aadhaar via DigiLocker',
  'masked_aadhaar': 'Masked Aadhaar',
  'birth_certificate': 'Birth certificate',
  'rent_agreement': 'Rent agreement',
  'utility_bill': 'Utility bill',
  'bank_statement': 'Bank statement',
  'voter_id': 'Voter ID',
  'employer_letter': 'Employer letter',
  'institution_id': 'School / college ID',
};

/// Submit a native or residence proof: notice → place → proof type →
/// (guardian) → upload → `POST /me/location-identities`.
class LocationIdentitySubmitScreen extends ConsumerWidget {
  const LocationIdentitySubmitScreen({super.key, required this.identityType});

  final String identityType;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(locationIdentitiesProvider);
    return Scaffold(
      appBar: AppBar(
        title: Text(identityType == IdentityType.native ? 'Native place proof' : 'Residence proof'),
      ),
      body: AsyncValueView<LocationIdentities>(
        value: value,
        onRetry: () => ref.invalidate(locationIdentitiesProvider),
        data: (ids) => _SubmitForm(identityType: identityType, ids: ids),
      ),
    );
  }
}

class _SubmitForm extends ConsumerStatefulWidget {
  const _SubmitForm({required this.identityType, required this.ids});

  final String identityType;
  final LocationIdentities ids;

  @override
  ConsumerState<_SubmitForm> createState() => _SubmitFormState();
}

class _SubmitFormState extends ConsumerState<_SubmitForm> {
  bool _noticeAccepted = false;
  int? _stateId;
  int? _districtId;
  String? _proofType;
  bool _guardianProof = false;
  UploadFile? _file;
  String? _fileError;

  bool get _isResidence => widget.identityType == IdentityType.residence;

  @override
  void initState() {
    super.initState();
    final current = widget.ids.slot(widget.identityType).current;
    // Renewing usually means the same place: start from the current one.
    _stateId = current?.state?.id ?? current?.district?.stateId;
    _districtId = current?.district?.id;
    final types = widget.ids.proofTypesFor(widget.identityType);
    if (types.length == 1) _proofType = types.single;
  }

  bool get _ready =>
      _noticeAccepted &&
      widget.ids.notice != null &&
      _districtId != null &&
      _proofType != null &&
      _file != null &&
      _fileError == null;

  Future<void> _pick() async {
    final f = await pickDocumentFile(extensions: _proofExtensions);
    if (f == null) return;
    setState(() {
      _file = f;
      _fileError = ref.read(mediaUploaderProvider).validate(MediaPurpose.locationProof, f);
    });
  }

  Future<void> _submit() async {
    final notice = widget.ids.notice!;
    try {
      await ref.read(locationIdentityControllerProvider).submit(
            identityType: widget.identityType,
            districtId: _districtId!,
            proofType: _proofType!,
            proof: _file!,
            guardianProof: _isResidence && _guardianProof,
            noticePolicyDocumentId: notice.policyDocumentId,
          );
      if (!mounted) return;
      _file = null; // drop the bytes
      showInfoSnack(context, 'Submitted. We will let you know once it is reviewed.');
      context.pop();
    } catch (e) {
      if (!mounted) return;
      await _handleError(e);
    }
  }

  Future<void> _handleError(Object e) async {
    switch (SubmitProblem.from(e)) {
      case CooldownProblem(:final nextChangeAt):
        await showDialog<void>(
          context: context,
          builder: (ctx) => AlertDialog(
            title: const Text('Too soon to change district'),
            content: Text(nextChangeAt == null
                ? 'You changed your residence district recently. You can renew for the same district at any time.'
                : 'You can change to a different district from ${formatDate(nextChangeAt)}. '
                    'You can renew for the same district at any time.'),
            actions: [TextButton(onPressed: () => Navigator.of(ctx).pop(), child: const Text('OK'))],
          ),
        );
      case PendingExistsProblem():
        await _offerWithdraw();
      case null:
        showErrorSnack(context, e);
    }
  }

  Future<void> _offerWithdraw() async {
    final ok = await confirm(
      context,
      title: 'A submission is already in review',
      body: 'Withdraw it to submit this one instead? The earlier proof is deleted.',
      ok: 'Withdraw',
    );
    if (!ok || !mounted) return;
    try {
      final fresh = await ref.refresh(locationIdentitiesProvider.future);
      final pending = fresh.slot(widget.identityType).pending;
      if (pending != null) {
        await ref.read(locationIdentityControllerProvider).withdraw(pending.id);
      }
      if (mounted) showInfoSnack(context, 'Withdrawn. You can submit now.');
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  @override
  Widget build(BuildContext context) {
    final ids = widget.ids;
    final notice = ids.notice;
    final types = ids.proofTypesFor(widget.identityType);
    final theme = Theme.of(context);

    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        // 1. Notice.
        Text('1. Read the notice', style: theme.textTheme.titleSmall),
        CheckboxListTile(
          contentPadding: EdgeInsets.zero,
          value: _noticeAccepted,
          onChanged: notice == null ? null : (v) => setState(() => _noticeAccepted = v ?? false),
          title: Text('I have read "${notice?.title ?? 'the location identity notice'}"'),
          subtitle: Align(
            alignment: Alignment.centerLeft,
            child: TextButton(
              style: TextButton.styleFrom(padding: EdgeInsets.zero),
              onPressed: () => context.push(Routes.policy(PolicyDocType.locationIdentityNotice)),
              child: const Text('Read notice'),
            ),
          ),
        ),
        if (notice == null)
          Text('The notice is not available right now. Try again later.',
              style: TextStyle(color: theme.colorScheme.error)),
        const SizedBox(height: 16),

        // 2. Place.
        Text(_isResidence ? '2. Where you live' : '2. Your native place', style: theme.textTheme.titleSmall),
        const SizedBox(height: 8),
        StateDropdown(
          value: _stateId,
          onChanged: (v) => setState(() {
            _stateId = v;
            _districtId = null;
          }),
        ),
        const SizedBox(height: 12),
        DistrictDropdown(
          stateId: _stateId,
          value: _districtId,
          onChanged: (v) => setState(() => _districtId = v),
        ),
        const SizedBox(height: 16),

        // 3. Proof type.
        Text('3. Proof type', style: theme.textTheme.titleSmall),
        const SizedBox(height: 8),
        EnumDropdown(
          label: 'Document',
          values: types,
          value: _proofType,
          labels: proofTypeLabels,
          onChanged: (v) => setState(() => _proofType = v),
        ),

        // 4. Guardian's proof (minors, residence only).
        if (_isResidence && ids.guardianProofAllowed)
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            value: _guardianProof,
            onChanged: (v) => setState(() => _guardianProof = v),
            title: const Text("This is my parent's or guardian's proof"),
            subtitle: const Text('Allowed for players under 18 who live with them.'),
          ),
        const SizedBox(height: 16),

        // 5. Upload.
        Text('4. Upload', style: theme.textTheme.titleSmall),
        const SizedBox(height: 8),
        Card(
          color: theme.colorScheme.secondaryContainer,
          child: const Padding(
            padding: EdgeInsets.all(12),
            child: Text(
              'Upload a masked Aadhaar only: the first 8 digits must be hidden and only the last 4 shown. '
              'Unmasked Aadhaar uploads are rejected and deleted. PDF, JPEG or PNG.',
            ),
          ),
        ),
        const SizedBox(height: 8),
        OutlinedButton.icon(
          onPressed: _pick,
          icon: const Icon(Icons.upload_file),
          label: Text(_file == null ? 'Choose file' : (_file!.name ?? 'File selected')),
        ),
        if (_fileError != null)
          Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Text(_fileError!, style: TextStyle(color: theme.colorScheme.error)),
          ),
        const SizedBox(height: 24),
        BusyButton(label: 'Submit for review', onPressed: _ready ? _submit : null),
      ],
    );
  }
}

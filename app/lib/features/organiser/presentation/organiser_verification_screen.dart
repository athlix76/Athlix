import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/format/dates.dart';
import '../../../core/media/pick_file.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/sport_picker.dart';
import '../application/organiser_controller.dart';
import '../data/organiser_models.dart';

/// `/organiser/verification`: organisation details plus the steps the server
/// defines (text, document upload, checkbox). Save as draft (PUT) or submit.
/// Editable only in `not_started | draft | rejected`.
class OrganiserVerificationScreen extends ConsumerWidget {
  const OrganiserVerificationScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final value = ref.watch(organiserVerificationProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Organiser verification')),
      body: AsyncValueView<OrganiserVerification>(
        value: value,
        onRetry: () => ref.invalidate(organiserVerificationProvider),
        // A new status rebuilds the form from the server's copy.
        data: (v) => _VerificationForm(key: ValueKey(v.status), verification: v),
      ),
    );
  }
}

class _VerificationForm extends ConsumerStatefulWidget {
  const _VerificationForm({super.key, required this.verification});

  final OrganiserVerification verification;

  @override
  ConsumerState<_VerificationForm> createState() => _VerificationFormState();
}

class _VerificationFormState extends ConsumerState<_VerificationForm> {
  late final _orgName = TextEditingController(text: widget.verification.orgName ?? '');
  late String? _orgType = widget.verification.orgType;
  final Map<int, TextEditingController> _text = {};
  final Map<int, bool> _checked = {};
  final Map<int, int> _docIds = {};

  /// File names picked in this session (the server only returns a signed URL).
  final Map<int, String> _docNames = {};
  final Set<int> _uploading = {};

  OrganiserVerification get v => widget.verification;

  @override
  void initState() {
    super.initState();
    for (final s in v.steps) {
      final sub = s.submission;
      switch (s.inputType) {
        case StepInputType.text:
          _text[s.stepId] = TextEditingController(text: sub?.valueText ?? '');
        case StepInputType.checkbox:
          _checked[s.stepId] = sub?.checked ?? false;
        case StepInputType.document:
          final id = sub?.document?.uploadId;
          if (id != null && id > 0) _docIds[s.stepId] = id;
      }
    }
  }

  @override
  void dispose() {
    _orgName.dispose();
    for (final c in _text.values) {
      c.dispose();
    }
    super.dispose();
  }

  List<StepInput> _stepInputs() => [
        for (final s in v.steps)
          switch (s.inputType) {
            StepInputType.text => StepInput(stepId: s.stepId, valueText: _text[s.stepId]?.text.trim()),
            StepInputType.checkbox => StepInput(stepId: s.stepId, checked: _checked[s.stepId] ?? false),
            StepInputType.document => StepInput(stepId: s.stepId, documentUploadId: _docIds[s.stepId]),
            _ => StepInput(stepId: s.stepId),
          },
      ];

  String? get _orgNameValue {
    final t = _orgName.text.trim();
    return t.isEmpty ? null : t;
  }

  /// Client-side hint only; the server decides on submit.
  List<String> _missing() => [
        if (_orgNameValue == null) 'Organisation name',
        if (_orgType == null) 'Organisation type',
        for (final s in v.steps)
          if (s.isRequired && !_filled(s)) s.title,
      ];

  bool _filled(VerificationStep s) => switch (s.inputType) {
        StepInputType.text => (_text[s.stepId]?.text.trim() ?? '').isNotEmpty,
        StepInputType.checkbox => _checked[s.stepId] ?? false,
        StepInputType.document => _docIds.containsKey(s.stepId),
        _ => true,
      };

  Future<void> _pickDocument(VerificationStep s) async {
    final allowed = ref.read(appConfigProvider).allowedDocumentTypes;
    final exts = <String>{
      for (final t in allowed ?? const ['application/pdf', 'image/jpeg', 'image/png']) ..._extensionsFor(t),
    }.toList();
    final file = await pickDocumentFile(extensions: exts.isEmpty ? const ['pdf', 'jpg', 'jpeg', 'png'] : exts);
    if (file == null || !mounted) return;
    setState(() => _uploading.add(s.stepId));
    try {
      final id = await ref.read(organiserVerificationProvider.notifier).uploadDocument(file);
      if (!mounted) return;
      setState(() {
        _docIds[s.stepId] = id;
        _docNames[s.stepId] = file.name ?? 'Document';
      });
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    } finally {
      if (mounted) setState(() => _uploading.remove(s.stepId));
    }
  }

  static List<String> _extensionsFor(String mime) => switch (mime) {
        'application/pdf' => const ['pdf'],
        'image/jpeg' => const ['jpg', 'jpeg'],
        'image/png' => const ['png'],
        'image/webp' => const ['webp'],
        _ => const <String>[],
      };

  Future<void> _saveDraft() async {
    await ref
        .read(organiserVerificationProvider.notifier)
        .saveDraft(orgName: _orgNameValue, orgType: _orgType, steps: _stepInputs());
    if (mounted) showInfoSnack(context, 'Draft saved.');
  }

  Future<void> _submit() async {
    final missing = _missing();
    if (missing.isNotEmpty) {
      showInfoSnack(context, 'Please complete: ${missing.join(', ')}');
      return;
    }
    final ok = await confirm(
      context,
      title: 'Submit for review?',
      body: 'You cannot change your answers while the review is in progress.',
      ok: 'Submit',
    );
    if (!ok) return;
    await ref
        .read(organiserVerificationProvider.notifier)
        .submit(orgName: _orgNameValue, orgType: _orgType, steps: _stepInputs());
    if (mounted) showInfoSnack(context, 'Submitted for review.');
  }

  @override
  Widget build(BuildContext context) {
    final editable = v.isEditable;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        _StatusBanner(verification: v),
        const SizedBox(height: 16),
        TextField(
          controller: _orgName,
          enabled: editable,
          decoration: const InputDecoration(labelText: 'Organisation name'),
        ),
        const SizedBox(height: 12),
        IgnorePointer(
          ignoring: !editable,
          child: EnumDropdown(
            label: 'Organisation type',
            values: OrgType.values,
            value: _orgType,
            onChanged: (t) => setState(() => _orgType = t),
          ),
        ),
        for (final s in v.steps) ...[
          const SizedBox(height: 20),
          _StepHeader(step: s),
          const SizedBox(height: 8),
          _stepInput(s, editable),
        ],
        const SizedBox(height: 24),
        if (editable)
          Row(
            children: [
              Expanded(
                child: BusyButton(
                  label: 'Save draft',
                  style: BusyButtonStyle.outlined,
                  onPressed: _uploading.isEmpty ? _saveDraft : null,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: BusyButton(label: 'Submit', onPressed: _uploading.isEmpty ? _submit : null),
              ),
            ],
          ),
      ],
    );
  }

  Widget _stepInput(VerificationStep s, bool editable) {
    switch (s.inputType) {
      case StepInputType.text:
        return TextField(
          controller: _text[s.stepId],
          enabled: editable,
          minLines: 1,
          maxLines: 4,
          onChanged: (_) => setState(() {}),
          decoration: InputDecoration(hintText: s.isRequired ? 'Required' : 'Optional'),
        );
      case StepInputType.checkbox:
        return CheckboxListTile(
          contentPadding: EdgeInsets.zero,
          controlAffinity: ListTileControlAffinity.leading,
          value: _checked[s.stepId] ?? false,
          title: Text(s.isRequired ? 'I confirm' : 'Yes'),
          onChanged: editable ? (on) => setState(() => _checked[s.stepId] = on ?? false) : null,
        );
      case StepInputType.document:
        final uploading = _uploading.contains(s.stepId);
        final hasDoc = _docIds.containsKey(s.stepId);
        final pickedName = _docNames[s.stepId];
        final storedUrl = pickedName == null ? s.submission?.document?.url : null;
        return Row(
          children: [
            Expanded(
              child: Text(
                pickedName != null
                    ? 'Attached: $pickedName'
                    : hasDoc
                        ? 'Document uploaded'
                        : 'No document yet',
              ),
            ),
            if (storedUrl != null && storedUrl.isNotEmpty)
              TextButton(
                onPressed: () => launchUrl(Uri.parse(storedUrl), mode: LaunchMode.externalApplication),
                child: const Text('View'),
              ),
            if (editable)
              uploading
                  ? const SizedBox(width: 24, height: 24, child: CircularProgressIndicator(strokeWidth: 2))
                  : OutlinedButton.icon(
                      onPressed: () => _pickDocument(s),
                      icon: const Icon(Icons.upload_file),
                      label: Text(hasDoc ? 'Replace' : 'Upload'),
                    ),
          ],
        );
      default:
        return const Text('Update the app to complete this step.');
    }
  }
}

class _StepHeader extends StatelessWidget {
  const _StepHeader({required this.step});

  final VerificationStep step;

  @override
  Widget build(BuildContext context) {
    final sub = step.submission;
    final theme = Theme.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Expanded(
              child: Text(
                step.isRequired ? '${step.title} *' : step.title,
                style: theme.textTheme.titleSmall,
              ),
            ),
            if (sub?.status != null) Chip(label: Text(humanize(sub!.status!))),
          ],
        ),
        if ((step.description ?? '').isNotEmpty) Text(step.description!, style: theme.textTheme.bodySmall),
        if ((sub?.reviewNote ?? '').isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Text('Reviewer: ${sub!.reviewNote!}', style: TextStyle(color: theme.colorScheme.error)),
          ),
      ],
    );
  }
}

class _StatusBanner extends StatelessWidget {
  const _StatusBanner({required this.verification});

  final OrganiserVerification verification;

  @override
  Widget build(BuildContext context) {
    final v = verification;
    final scheme = Theme.of(context).colorScheme;
    final (IconData icon, String text, Color color) = switch (v.status) {
      OrganiserStatus.approved => (Icons.verified, 'You are a verified organiser.', scheme.primary),
      OrganiserStatus.submitted => (
          Icons.hourglass_top,
          'Submitted ${formatDate(v.submittedAt)}. We will notify you after the review.',
          scheme.tertiary
        ),
      OrganiserStatus.rejected => (Icons.error_outline, 'Changes needed. Update and submit again.', scheme.error),
      OrganiserStatus.draft => (Icons.edit_note, 'Draft saved. Submit when every required step is done.', scheme.secondary),
      _ => (Icons.info_outline, 'Verify your organisation to list events.', scheme.secondary),
    };
    return Card(
      child: ListTile(
        leading: Icon(icon, color: color),
        title: Text(text),
        subtitle: (v.reviewNote ?? '').isEmpty ? null : Text(v.reviewNote!),
      ),
    );
  }
}

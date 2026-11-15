import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/format/dates.dart';
import '../../../core/media/media_upload.dart';
import '../../../core/media/pick_file.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/sport_picker.dart';
import '../application/profile_controller.dart';
import '../data/profile_models.dart';

/// File types offered for a certificate. The server and `media.*` config decide what is accepted.
const _certificateExtensions = ['pdf', 'jpg', 'jpeg', 'png'];

/// Add or edit an achievement, with an optional `achievement_certificate` upload.
Future<void> showAchievementSheet(BuildContext context, {Achievement? existing}) {
  return showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    showDragHandle: true,
    builder: (_) => _AchievementSheet(existing: existing),
  );
}

class _AchievementSheet extends ConsumerStatefulWidget {
  const _AchievementSheet({this.existing});

  final Achievement? existing;

  @override
  ConsumerState<_AchievementSheet> createState() => _AchievementSheetState();
}

class _AchievementSheetState extends ConsumerState<_AchievementSheet> {
  late int? _sportId = widget.existing?.sportId;
  late final _title = TextEditingController(text: widget.existing?.title ?? '');
  late final _description = TextEditingController(text: widget.existing?.description ?? '');
  late String _level = widget.existing?.level ?? AchievementLevel.all.first;
  late DateTime? _achievedOn = widget.existing?.achievedOn;
  UploadFile? _certificate;

  @override
  void dispose() {
    _title.dispose();
    _description.dispose();
    super.dispose();
  }

  Future<void> _pickCertificate() async {
    final file = await pickDocumentFile(extensions: _certificateExtensions);
    if (file == null || !mounted) return;
    final problem = ref.read(profileControllerProvider).validate(MediaPurpose.achievementCertificate, file);
    if (problem != null) {
      showInfoSnack(context, problem);
      return;
    }
    setState(() => _certificate = file);
  }

  Future<void> _pickDate() async {
    final now = DateTime.now();
    final d = await pickDate(context, initial: _achievedOn ?? now, last: now);
    if (d != null) setState(() => _achievedOn = d);
  }

  Future<void> _save() async {
    final title = _title.text.trim();
    final date = _achievedOn;
    if (_sportId == null || title.isEmpty || date == null) {
      showInfoSnack(context, 'Sport, title and date are required.');
      return;
    }
    final controller = ref.read(profileControllerProvider);
    final cert = _certificate;
    final uploadId = cert == null ? null : await controller.upload(MediaPurpose.achievementCertificate, cert);
    final description = _description.text.trim();
    await controller.saveAchievement(
      AchievementInput(
        sportId: _sportId,
        title: title,
        description: description.isEmpty ? null : description,
        level: _level,
        achievedOn: date,
        certificateUploadId: uploadId,
      ),
      id: widget.existing?.id,
    );
    if (mounted) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    const gap = SizedBox(height: 12);
    final bottom = MediaQuery.of(context).viewInsets.bottom;
    final hasExistingCert = widget.existing?.certificateUrl != null;
    return Padding(
      padding: EdgeInsets.fromLTRB(16, 0, 16, 16 + bottom),
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(widget.existing == null ? 'Add achievement' : 'Edit achievement',
                style: Theme.of(context).textTheme.titleLarge),
            gap,
            SportDropdown(value: _sportId, onChanged: (v) => setState(() => _sportId = v)),
            gap,
            TextField(controller: _title, maxLength: 120, decoration: const InputDecoration(labelText: 'Title')),
            TextField(
              controller: _description,
              maxLength: 500,
              minLines: 2,
              maxLines: 4,
              decoration: const InputDecoration(labelText: 'Description (optional)'),
            ),
            gap,
            EnumDropdown(
              label: 'Level',
              values: AchievementLevel.all,
              value: _level,
              onChanged: (v) => setState(() => _level = v ?? _level),
            ),
            gap,
            OutlinedButton.icon(
              onPressed: _pickDate,
              icon: const Icon(Icons.calendar_today_outlined),
              label: Text(_achievedOn == null ? 'Date achieved' : formatDate(_achievedOn)),
            ),
            gap,
            OutlinedButton.icon(
              onPressed: _pickCertificate,
              icon: const Icon(Icons.attach_file),
              label: Text(
                _certificate != null
                    ? (_certificate!.name ?? 'Certificate selected')
                    : hasExistingCert
                        ? 'Replace certificate'
                        : 'Attach certificate (optional)',
                overflow: TextOverflow.ellipsis,
              ),
            ),
            const SizedBox(height: 20),
            BusyButton(label: 'Save', onPressed: _save),
          ],
        ),
      ),
    );
  }
}

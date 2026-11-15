import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/media/pick_image.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/sport_picker.dart';
import '../application/onboarding_controller.dart';
import '../data/onboarding_api.dart';

/// Onboarding step `profile` (last step).
class ProfileSetupScreen extends ConsumerStatefulWidget {
  const ProfileSetupScreen({super.key});

  @override
  ConsumerState<ProfileSetupScreen> createState() => _ProfileSetupScreenState();
}

class _ProfileSetupScreenState extends ConsumerState<ProfileSetupScreen> {
  final _form = GlobalKey<FormState>();
  final _name = TextEditingController();
  final _city = TextEditingController();
  final _district = TextEditingController();
  final _state = TextEditingController();
  final _bio = TextEditingController();
  final _club = TextEditingController();
  int? _sportId;
  int? _positionId;
  int? _photoUploadId;
  bool _uploading = false;

  @override
  void initState() {
    super.initState();
    _name.text = ref.read(meProvider)?.displayName ?? '';
  }

  @override
  void dispose() {
    for (final c in [_name, _city, _district, _state, _bio, _club]) {
      c.dispose();
    }
    super.dispose();
  }

  String? _required(String? v) => (v == null || v.trim().isEmpty) ? 'Required' : null;

  String? _opt(TextEditingController c) => c.text.trim().isEmpty ? null : c.text.trim();

  Future<void> _pickPhoto() async {
    final file = await pickImageFile();
    if (file == null) return;
    setState(() => _uploading = true);
    try {
      final id = await ref.read(onboardingControllerProvider).uploadPhoto(file);
      if (mounted) setState(() => _photoUploadId = id);
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    } finally {
      if (mounted) setState(() => _uploading = false);
    }
  }

  Future<void> _submit() async {
    if (!(_form.currentState?.validate() ?? false)) return;
    final sportId = _sportId;
    if (sportId == null) {
      showInfoSnack(context, 'Choose your sport');
      return;
    }
    await ref.read(onboardingControllerProvider).completeProfile(ProfileSetup(
          displayName: _name.text.trim(),
          sportId: sportId,
          positionId: _positionId,
          city: _city.text.trim(),
          district: _opt(_district),
          state: _state.text.trim(),
          bio: _opt(_bio),
          academyClub: _opt(_club),
          photoUploadId: _photoUploadId,
        ));
  }

  @override
  Widget build(BuildContext context) {
    final role = ref.watch(meProvider)?.primaryRole;
    return Scaffold(
      appBar: AppBar(title: const Text('Your profile')),
      body: Form(
        key: _form,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            Center(
              child: OutlinedButton.icon(
                onPressed: _uploading ? null : _pickPhoto,
                icon: Icon(_photoUploadId == null ? Icons.add_a_photo : Icons.check_circle),
                label: Text(_uploading
                    ? 'Uploading...'
                    : (_photoUploadId == null ? 'Add a photo (optional)' : 'Photo added')),
              ),
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _name,
              decoration: const InputDecoration(labelText: 'Display name'),
              validator: _required,
            ),
            const SizedBox(height: 12),
            SportDropdown(
              value: _sportId,
              onChanged: (v) => setState(() {
                _sportId = v;
                _positionId = null;
              }),
            ),
            const SizedBox(height: 12),
            PositionDropdown(
              sportId: _sportId,
              value: _positionId,
              role: role,
              onChanged: (v) => setState(() => _positionId = v),
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _city,
              decoration: const InputDecoration(labelText: 'City'),
              validator: _required,
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _district,
              decoration: const InputDecoration(labelText: 'District (optional)'),
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _state,
              decoration: const InputDecoration(labelText: 'State'),
              validator: _required,
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _club,
              decoration: const InputDecoration(labelText: 'Academy / club (optional)'),
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _bio,
              maxLines: 3,
              decoration: const InputDecoration(labelText: 'Bio (optional)'),
            ),
            const SizedBox(height: 24),
            BusyButton(label: 'Finish', onPressed: _uploading ? null : _submit),
          ],
        ),
      ),
    );
  }
}

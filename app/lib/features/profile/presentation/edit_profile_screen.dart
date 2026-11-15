import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/media/media_upload.dart';
import '../../../core/media/pick_image.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/section.dart';
import '../../../core/widgets/sport_picker.dart';
import '../../../core/widgets/user_avatar.dart';
import '../application/profile_controller.dart';
import '../application/profile_providers.dart';
import '../data/profile_models.dart';

/// Edit the signed-in user's profile (`PUT /me/profile`).
class EditProfileScreen extends ConsumerWidget {
  const EditProfileScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final profile = ref.watch(myProfileProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('Edit profile')),
      body: AsyncValueView<Profile?>(
        value: profile,
        onRetry: () => ref.invalidate(myProfileProvider),
        data: (p) => p == null ? const EmptyView(text: 'Sign in to edit your profile.') : _EditForm(profile: p),
      ),
    );
  }
}

class _SportRow {
  _SportRow({this.sportId, this.positionId});

  int? sportId;
  int? positionId;
}

class _EditForm extends ConsumerStatefulWidget {
  const _EditForm({required this.profile});

  final Profile profile;

  @override
  ConsumerState<_EditForm> createState() => _EditFormState();
}

class _EditFormState extends ConsumerState<_EditForm> {
  late final Profile _p = widget.profile;
  late final _name = TextEditingController(text: _p.user.displayName);
  late final _bio = TextEditingController(text: _p.bio ?? '');
  late final _city = TextEditingController(text: _p.user.city ?? '');
  late final _district = TextEditingController(text: _p.district ?? '');
  late final _state = TextEditingController(text: _p.user.state ?? '');
  late final _club = TextEditingController(text: _p.academyClub ?? '');
  late final _coachingSince = TextEditingController(text: _p.coachingSinceYear?.toString() ?? '');
  late String? _visibility = _p.visibility;
  late String? _statsVisibility = _p.statsVisibility;
  late final List<_SportRow> _sports = [
    for (final s in _p.sports) _SportRow(sportId: s.sport.id, positionId: s.position?.id),
  ];
  late int _primary = () {
    final i = _p.sports.indexWhere((s) => s.isPrimary);
    return i < 0 ? 0 : i;
  }();
  UploadFile? _photo;

  @override
  void dispose() {
    for (final c in [_name, _bio, _city, _district, _state, _club, _coachingSince]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _pickPhoto() async {
    final file = await pickImageFile();
    if (file == null || !mounted) return;
    final problem = ref.read(profileControllerProvider).validate(MediaPurpose.profilePhoto, file);
    if (problem != null) {
      showInfoSnack(context, problem);
      return;
    }
    setState(() => _photo = file);
  }

  Future<void> _save() async {
    final name = _name.text.trim();
    if (name.isEmpty) {
      showInfoSnack(context, 'Enter your name.');
      return;
    }
    final rows = _sports.where((r) => r.sportId != null).toList();
    final chosen = _primary < _sports.length ? _sports[_primary] : null;
    final primaryRow = rows.contains(chosen) ? chosen : (rows.isEmpty ? null : rows.first);
    final controller = ref.read(profileControllerProvider);
    final photo = _photo;
    final photoUploadId = photo == null ? null : await controller.upload(MediaPurpose.profilePhoto, photo);
    final isCoach = ref.read(meProvider)?.isCoach ?? false;
    await controller.updateProfile(ProfileUpdate(
      displayName: name,
      bio: _bio.text.trim(),
      sports: [
        for (final r in rows)
          ProfileSportInput(
            sportId: r.sportId!,
            positionId: r.positionId,
            isPrimary: identical(r, primaryRow),
          ),
      ],
      city: _city.text.trim(),
      district: _district.text.trim(),
      state: _state.text.trim(),
      academyClub: _club.text.trim(),
      photoUploadId: photoUploadId,
      visibility: _visibility,
      statsVisibility: _statsVisibility,
      coachingSinceYear: isCoach ? int.tryParse(_coachingSince.text.trim()) : null,
    ));
    if (!mounted) return;
    showInfoSnack(context, 'Profile saved.');
    context.pop();
  }

  @override
  Widget build(BuildContext context) {
    final me = ref.watch(meProvider);
    final config = ref.watch(appConfigProvider);
    // Minors cannot choose "public" unless the minor default allows it (server returns 422 otherwise).
    final minorLocked = (me?.isMinor ?? false) && config.minorProfileDefaultVisibility != ProfileVisibility.public;
    final visibilityValues = [
      for (final v in ProfileVisibility.all)
        if (!(minorLocked && v == ProfileVisibility.public)) v,
    ];
    const gap = SizedBox(height: 12);
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 32),
      children: [
        Center(
          child: Column(
            children: [
              _photo != null
                  ? CircleAvatar(radius: 44, backgroundImage: MemoryImage(_photo!.bytes))
                  : UserAvatar(name: _p.user.displayName, url: _p.user.photoUrl, radius: 44),
              TextButton.icon(
                onPressed: _pickPhoto,
                icon: const Icon(Icons.photo_camera_outlined),
                label: const Text('Change photo'),
              ),
            ],
          ),
        ),
        TextField(controller: _name, maxLength: 60, decoration: const InputDecoration(labelText: 'Name')),
        TextField(
          controller: _bio,
          maxLength: 500,
          minLines: 2,
          maxLines: 5,
          decoration: const InputDecoration(labelText: 'Bio'),
        ),
        const SectionHeader('Sports'),
        for (var i = 0; i < _sports.length; i++) _sportRow(i, me?.primaryRole),
        Align(
          alignment: Alignment.centerLeft,
          child: TextButton.icon(
            onPressed: () => setState(() => _sports.add(_SportRow())),
            icon: const Icon(Icons.add),
            label: const Text('Add sport'),
          ),
        ),
        const SectionHeader('Location and club'),
        TextField(controller: _city, decoration: const InputDecoration(labelText: 'City')),
        gap,
        TextField(controller: _district, decoration: const InputDecoration(labelText: 'District')),
        gap,
        TextField(controller: _state, decoration: const InputDecoration(labelText: 'State')),
        gap,
        TextField(controller: _club, decoration: const InputDecoration(labelText: 'Academy / club')),
        if (me?.isCoach ?? false) ...[
          gap,
          TextField(
            controller: _coachingSince,
            keyboardType: TextInputType.number,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly, LengthLimitingTextInputFormatter(4)],
            decoration: const InputDecoration(labelText: 'Coaching since (year)'),
          ),
        ],
        const SectionHeader('Privacy'),
        EnumDropdown(
          label: 'Who can see my profile',
          values: visibilityValues,
          labels: ProfileVisibility.labels,
          value: _visibility,
          onChanged: (v) => setState(() => _visibility = v),
        ),
        if (minorLocked)
          const Padding(
            padding: EdgeInsets.only(top: 4),
            child: Text('Profiles of players under 18 cannot be public.'),
          ),
        gap,
        EnumDropdown(
          label: 'Who can see my stats',
          values: ProfileVisibility.all,
          labels: ProfileVisibility.labels,
          value: _statsVisibility,
          onChanged: (v) => setState(() => _statsVisibility = v),
        ),
        const SizedBox(height: 24),
        BusyButton(label: 'Save', onPressed: _save),
      ],
    );
  }

  Widget _sportRow(int i, String? role) {
    final row = _sports[i];
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 12, 4, 4),
        child: Column(
          children: [
            Row(
              children: [
                Expanded(
                  child: SportDropdown(
                    value: row.sportId,
                    onChanged: (v) => setState(() {
                      row.sportId = v;
                      row.positionId = null;
                    }),
                  ),
                ),
                IconButton(
                  tooltip: 'Remove',
                  icon: const Icon(Icons.close),
                  onPressed: () => setState(() {
                    _sports.removeAt(i);
                    if (_primary >= _sports.length) _primary = 0;
                  }),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Padding(
              padding: const EdgeInsets.only(right: 8),
              child: PositionDropdown(
                sportId: row.sportId,
                value: row.positionId,
                role: role,
                onChanged: (v) => setState(() => row.positionId = v),
              ),
            ),
            RadioListTile<int>(
              value: i,
              groupValue: _primary,
              contentPadding: EdgeInsets.zero,
              title: const Text('Primary sport'),
              onChanged: (v) => setState(() => _primary = v ?? 0),
            ),
          ],
        ),
      ),
    );
  }
}

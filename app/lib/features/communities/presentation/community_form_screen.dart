import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/media/media_upload.dart';
import '../../../core/media/pick_image.dart';
import '../../../core/router/routes.dart';
import '../../../core/widgets/async_value_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/pickers.dart';
import '../../../core/widgets/sport_picker.dart';
import '../../../core/widgets/user_avatar.dart';
import '../application/community_providers.dart';
import '../data/community_models.dart';

/// Create (no [communityId]) or edit a community. Edit sends only changed fields.
class CommunityFormScreen extends ConsumerWidget {
  const CommunityFormScreen({super.key, this.communityId});

  final int? communityId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final id = communityId;
    if (id == null) return const _Form(existing: null);
    return AsyncValueView<Community>(
      value: ref.watch(communityProvider(id)),
      onRetry: () => ref.invalidate(communityProvider(id)),
      data: (c) => _Form(existing: c),
    );
  }
}

class _Form extends ConsumerStatefulWidget {
  const _Form({required this.existing});

  final Community? existing;

  @override
  ConsumerState<_Form> createState() => _FormState();
}

class _FormState extends ConsumerState<_Form> {
  final _key = GlobalKey<FormState>();
  late final _title = TextEditingController(text: widget.existing?.title ?? '');
  late final _description = TextEditingController(text: widget.existing?.description ?? '');
  late final _city = TextEditingController(text: widget.existing?.city ?? '');
  late final _state = TextEditingController(text: widget.existing?.state ?? '');
  late int? _sportId = widget.existing?.sport?.id;
  late String _visibility = widget.existing?.visibility ?? CommunityVisibility.public;
  late String _joinPolicy = widget.existing?.joinPolicy ?? JoinPolicy.open;
  UploadFile? _avatar;

  bool get _isEdit => widget.existing != null;

  @override
  void dispose() {
    _title.dispose();
    _description.dispose();
    _city.dispose();
    _state.dispose();
    super.dispose();
  }

  static String? _orNull(String s) => s.trim().isEmpty ? null : s.trim();

  Future<void> _pickAvatar() async {
    final file = await pickImageFile();
    if (file == null || !mounted) return;
    final problem = ref.read(mediaUploaderProvider).validate(MediaPurpose.communityAvatar, file);
    if (problem != null) {
      showInfoSnack(context, problem);
      return;
    }
    setState(() => _avatar = file);
  }

  Future<void> _save() async {
    if (!(_key.currentState?.validate() ?? false)) return;
    final ctl = ref.read(communityControllerProvider);
    final avatar = _avatar;
    final uploadId = avatar == null ? null : await ctl.uploadAvatar(avatar);
    final input = CommunityInput(
      title: _title.text.trim(),
      description: _orNull(_description.text),
      avatarUploadId: uploadId,
      sportId: _sportId,
      city: _orNull(_city.text),
      state: _orNull(_state.text),
      visibility: _visibility,
      joinPolicy: _joinPolicy,
    );
    final existing = widget.existing;
    if (existing == null) {
      final c = await ctl.create(input);
      if (mounted) context.pushReplacement(Routes.community(c.id));
      return;
    }
    final changed = <String>{
      if (input.title != existing.title) 'title',
      if (input.description != existing.description) 'description',
      if (uploadId != null) 'avatarUploadId',
      if (input.sportId != existing.sport?.id) 'sportId',
      if (input.city != existing.city) 'city',
      if (input.state != existing.state) 'state',
      if (input.visibility != existing.visibility) 'visibility',
      if (input.joinPolicy != existing.joinPolicy) 'joinPolicy',
    };
    if (changed.isNotEmpty) await ctl.update(existing.id, input.toJson(only: changed));
    if (mounted) context.pop();
  }

  Future<void> _delete() async {
    final c = widget.existing!;
    final ok = await confirm(
      context,
      title: 'Delete ${c.title}?',
      body: 'The community and its chat will be archived for everyone.',
      ok: 'Delete',
    );
    if (!ok || !mounted) return;
    await ref.read(communityControllerProvider).delete(c.id);
    if (mounted) context.go(Routes.communities);
  }

  @override
  Widget build(BuildContext context) {
    final existing = widget.existing;
    final config = ref.watch(appConfigProvider);
    final maxCreated = config.maxCommunitiesCreatedPerUser;
    final maxMembers = config.chatGroupMaxMembers;
    final avatar = _avatar;
    return Scaffold(
      appBar: AppBar(title: Text(_isEdit ? 'Edit community' : 'New community')),
      body: Form(
        key: _key,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            Center(
              child: InkWell(
                customBorder: const CircleBorder(),
                onTap: _pickAvatar,
                child: Stack(
                  alignment: Alignment.bottomRight,
                  children: [
                    avatar != null
                        ? CircleAvatar(radius: 44, backgroundImage: MemoryImage(avatar.bytes))
                        : UserAvatar(
                            name: _title.text.isEmpty ? '?' : _title.text,
                            url: existing?.avatarUrl,
                            radius: 44,
                          ),
                    const CircleAvatar(radius: 14, child: Icon(Icons.camera_alt_outlined, size: 16)),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 16),
            TextFormField(
              controller: _title,
              maxLength: 80,
              decoration: const InputDecoration(labelText: 'Name'),
              validator: (v) => (v == null || v.trim().isEmpty) ? 'Required' : null,
            ),
            const SizedBox(height: 8),
            TextFormField(
              controller: _description,
              minLines: 2,
              maxLines: 5,
              maxLength: 1000,
              decoration: const InputDecoration(labelText: 'Description'),
            ),
            const SizedBox(height: 8),
            SportDropdown(value: _sportId, allowAny: true, onChanged: (v) => setState(() => _sportId = v)),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: TextFormField(controller: _city, decoration: const InputDecoration(labelText: 'City')),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: TextFormField(controller: _state, decoration: const InputDecoration(labelText: 'State')),
                ),
              ],
            ),
            const SizedBox(height: 12),
            EnumDropdown(
              label: 'Visibility',
              values: CommunityVisibility.values,
              value: _visibility,
              labels: const {
                CommunityVisibility.public: 'Public (anyone can find it)',
                CommunityVisibility.private: 'Private (members and invited only)',
              },
              onChanged: (v) => setState(() => _visibility = v ?? _visibility),
            ),
            const SizedBox(height: 12),
            EnumDropdown(
              label: 'Who can join',
              values: JoinPolicy.values,
              value: _joinPolicy,
              labels: const {
                JoinPolicy.open: 'Anyone',
                JoinPolicy.approval: 'Anyone, after admin approval',
                JoinPolicy.invite: 'Invited people only',
              },
              onChanged: (v) => setState(() => _joinPolicy = v ?? _joinPolicy),
            ),
            const SizedBox(height: 12),
            if (!_isEdit && maxCreated != null)
              Text('You can create up to $maxCreated communities.', style: Theme.of(context).textTheme.bodySmall),
            if (maxMembers != null)
              Text('A community can have up to $maxMembers members.', style: Theme.of(context).textTheme.bodySmall),
            const SizedBox(height: 16),
            BusyButton(label: _isEdit ? 'Save' : 'Create', onPressed: _save),
            if (existing != null && existing.isOwner) ...[
              const SizedBox(height: 24),
              BusyButton(label: 'Delete community', style: BusyButtonStyle.outlined, icon: Icons.delete_outline, onPressed: _delete),
            ],
          ],
        ),
      ),
    );
  }
}

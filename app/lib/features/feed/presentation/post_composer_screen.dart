import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/config_providers.dart';
import '../../../core/media/media_upload.dart';
import '../../../core/media/pick_image.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/widgets/sport_picker.dart';
import '../application/feed_controller.dart';
import '../data/post_models.dart';

/// `/posts/new?eventId=`: write a post with up to `feed.post_max_media`
/// images (`post_image` uploads). Pops with the created [Post].
class PostComposerScreen extends ConsumerStatefulWidget {
  const PostComposerScreen({super.key, this.eventId});

  /// Set when posting about an event (from the event screen).
  final int? eventId;

  @override
  ConsumerState<PostComposerScreen> createState() => _PostComposerScreenState();
}

class _PostComposerScreenState extends ConsumerState<PostComposerScreen> {
  final _body = TextEditingController();
  final List<UploadFile> _images = [];
  int? _sportId;
  String _visibility = PostVisibility.public;

  @override
  void initState() {
    super.initState();
    _body.addListener(() => setState(() {}));
    // Same default as the server for minors (docs/api.md "Feed").
    final me = ref.read(meProvider);
    final minorDefault = ref.read(appConfigProvider).minorProfileDefaultVisibility;
    if ((me?.isMinor ?? false) && minorDefault != null && minorDefault != 'public') {
      _visibility = PostVisibility.followers;
    }
  }

  @override
  void dispose() {
    _body.dispose();
    super.dispose();
  }

  /// Null = config not loaded yet; the server enforces the limit anyway.
  int? get _maxMedia => ref.read(appConfigProvider).postMaxMedia;

  Future<void> _addImages() async {
    final max = _maxMedia;
    final remaining = max == null ? null : max - _images.length;
    if (remaining != null && remaining <= 0) {
      showInfoSnack(context, 'You can add up to $max images.');
      return;
    }
    try {
      final picked = await pickImageFiles(limit: remaining);
      if (picked.isEmpty || !mounted) return;
      final uploader = ref.read(mediaUploaderProvider);
      final problems = <String>{};
      for (final f in picked) {
        final problem = uploader.validate(MediaPurpose.postImage, f);
        if (problem == null) {
          _images.add(f);
        } else {
          problems.add(problem);
        }
      }
      setState(() {});
      if (problems.isNotEmpty) showInfoSnack(context, problems.join('\n'));
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  bool get _canPost => _body.text.trim().isNotEmpty || _images.isNotEmpty;

  Future<void> _submit() async {
    final post = await ref.read(feedControllerProvider).create(
          body: _body.text.trim(),
          images: List.of(_images),
          sportId: _sportId,
          eventId: widget.eventId,
          visibility: _visibility,
        );
    if (mounted) context.pop(post);
  }

  @override
  Widget build(BuildContext context) {
    final config = ref.watch(appConfigProvider);
    final maxMedia = config.postMaxMedia;
    final mediaAllowed = maxMedia == null || maxMedia > 0;
    return Scaffold(
      appBar: AppBar(
        title: const Text('New post'),
        actions: [
          Padding(
            padding: const EdgeInsets.only(right: 8),
            child: BusyButton(label: 'Post', onPressed: _canPost ? _submit : null),
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          if (widget.eventId != null)
            const Padding(
              padding: EdgeInsets.only(bottom: 12),
              child: Chip(avatar: Icon(Icons.event, size: 18), label: Text('About an event')),
            ),
          TextField(
            controller: _body,
            maxLength: config.postMaxLength,
            minLines: 4,
            maxLines: 10,
            decoration: const InputDecoration(hintText: 'Share something with your sports community'),
          ),
          const SizedBox(height: 12),
          if (_images.isNotEmpty)
            SizedBox(
              height: 96,
              child: ListView.separated(
                scrollDirection: Axis.horizontal,
                itemCount: _images.length,
                separatorBuilder: (_, __) => const SizedBox(width: 8),
                itemBuilder: (_, i) => Stack(
                  children: [
                    ClipRRect(
                      borderRadius: BorderRadius.circular(8),
                      child: Image.memory(_images[i].bytes, width: 96, height: 96, fit: BoxFit.cover),
                    ),
                    Positioned(
                      right: 0,
                      top: 0,
                      child: IconButton.filledTonal(
                        visualDensity: VisualDensity.compact,
                        iconSize: 16,
                        tooltip: 'Remove',
                        onPressed: () => setState(() => _images.removeAt(i)),
                        icon: const Icon(Icons.close),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          if (mediaAllowed)
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton.icon(
                onPressed: maxMedia != null && _images.length >= maxMedia ? null : _addImages,
                icon: const Icon(Icons.add_photo_alternate_outlined),
                label: Text(maxMedia == null ? 'Add images' : 'Add images (${_images.length}/$maxMedia)'),
              ),
            ),
          const SizedBox(height: 12),
          SportDropdown(
            value: _sportId,
            label: 'Sport (optional)',
            allowAny: true,
            onChanged: (v) => setState(() => _sportId = v),
          ),
          const SizedBox(height: 12),
          EnumDropdown(
            label: 'Who can see this',
            values: PostVisibility.values,
            labels: PostVisibility.labels,
            value: _visibility,
            onChanged: (v) => setState(() => _visibility = v ?? PostVisibility.public),
          ),
        ],
      ),
    );
  }
}

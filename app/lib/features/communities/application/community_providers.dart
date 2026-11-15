import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/media/media_upload.dart';
import '../../chat/application/chat_socket_provider.dart';
import '../../chat/data/chat_api.dart';
import '../data/community_api.dart';
import '../data/community_models.dart';

final communityProvider = FutureProvider.autoDispose.family<Community, int>(
  (ref, id) => ref.watch(communityApiProvider).get(id),
);

/// Filters of the Discover tab (kept while the app runs).
typedef CommunityFilters = ({String q, int? sportId, String state});

final communityFiltersProvider = StateProvider<CommunityFilters>((ref) => (q: '', sportId: null, state: ''));

/// Writes for communities. Every write refreshes the cached community (and
/// its conversation, which shares the id).
class CommunityController {
  CommunityController(this._ref);

  final Ref _ref;

  CommunityApi get _api => _ref.read(communityApiProvider);

  void _refresh(int id) {
    _ref.invalidate(communityProvider(id));
    _ref.invalidate(conversationProvider(id));
  }

  /// Upload a new avatar as `community_avatar`; returns the `uploadId`.
  Future<int> uploadAvatar(UploadFile file) =>
      _ref.read(mediaUploaderProvider).upload(MediaPurpose.communityAvatar, file);

  Future<Community> create(CommunityInput input) => _api.create(input);

  Future<Community> update(int id, Map<String, dynamic> body) async {
    final c = await _api.update(id, body);
    _refresh(id);
    return c;
  }

  Future<void> delete(int id) async {
    await _api.delete(id);
    _refresh(id);
  }

  /// Returns the new status: `active` (joined) or `pending` (request sent).
  Future<String> join(int id) async {
    final status = await _api.join(id);
    _refresh(id);
    return status;
  }

  Future<void> acceptInvite(int id) async {
    await _api.acceptInvite(id);
    _refresh(id);
  }

  /// Leaving uses the chat endpoint (a community is a conversation).
  Future<void> leave(int id) async {
    await _ref.read(chatApiProvider).leave(id);
    _refresh(id);
  }

  Future<void> approve(int id, int userId) async {
    await _api.approve(id, userId);
    _refresh(id);
  }

  Future<void> reject(int id, int userId) => _api.reject(id, userId);

  Future<InviteResult> invite(int id, List<int> userIds) => _api.invite(id, userIds);

  Future<void> removeMember(int id, int userId) async {
    await _api.removeMember(id, userId);
    _refresh(id);
  }

  Future<void> banMember(int id, int userId) async {
    await _api.banMember(id, userId);
    _refresh(id);
  }

  Future<void> setRole(int id, int userId, String role) => _api.setRole(id, userId, role);

  Future<void> transferOwnership(int id, int userId) async {
    await _api.transferOwnership(id, userId);
    _refresh(id);
  }
}

final communityControllerProvider = Provider<CommunityController>(CommunityController.new);

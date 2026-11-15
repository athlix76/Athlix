import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/media/media_upload.dart';
import '../../../core/session/session_controller.dart';
import '../data/organiser_api.dart';
import '../data/organiser_models.dart';

/// Organiser verification state. Writes replace the state with the server's
/// answer, so step statuses and review notes stay in sync.
class OrganiserVerificationController extends AutoDisposeAsyncNotifier<OrganiserVerification> {
  @override
  Future<OrganiserVerification> build() {
    ref.watch(currentUserIdProvider);
    return ref.watch(organiserApiProvider).get();
  }

  OrganiserApi get _api => ref.read(organiserApiProvider);

  /// Uploads one document (`organiser_document`) and returns its upload id.
  /// The id is saved with the next [saveDraft].
  Future<int> uploadDocument(UploadFile file) =>
      ref.read(mediaUploaderProvider).upload(MediaPurpose.organiserDocument, file);

  Future<void> saveDraft({String? orgName, String? orgType, required List<StepInput> steps}) async {
    state = AsyncData(await _api.save(orgName: orgName, orgType: orgType, steps: steps));
  }

  /// Saves the draft first, then submits.
  Future<void> submit({String? orgName, String? orgType, required List<StepInput> steps}) async {
    await saveDraft(orgName: orgName, orgType: orgType, steps: steps);
    state = AsyncData(await _api.submit());
    await ref.read(sessionProvider.notifier).refreshMe();
  }
}

final organiserVerificationProvider =
    AsyncNotifierProvider.autoDispose<OrganiserVerificationController, OrganiserVerification>(
        OrganiserVerificationController.new);

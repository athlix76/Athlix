import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/media/media_upload.dart';
import '../../../core/session/session_controller.dart';
import '../data/onboarding_api.dart';

/// Runs onboarding calls and hands the returned `Me` to the session, which
/// moves the router to the next step.
class OnboardingController {
  OnboardingController(this._ref);

  final Ref _ref;

  Future<void> chooseRole(String role, List<String> additional) async {
    final me = await _ref
        .read(onboardingApiProvider)
        .setRole(role, additionalRoles: additional.where((r) => r != role).toList());
    _ref.read(sessionProvider.notifier).setMe(me);
  }

  Future<int> uploadPhoto(UploadFile file) =>
      _ref.read(mediaUploaderProvider).upload(MediaPurpose.profilePhoto, file);

  Future<void> completeProfile(ProfileSetup setup) async {
    final me = await _ref.read(onboardingApiProvider).completeProfile(setup);
    _ref.read(sessionProvider.notifier).setMe(me);
  }
}

final onboardingControllerProvider = Provider<OnboardingController>(OnboardingController.new);

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_exception.dart';
import '../../../core/media/media_upload.dart';
import '../data/geo_models.dart';
import '../data/location_identity_api.dart';
import '../data/location_identity_models.dart';

/// Active states (cached for the app run; the list rarely changes).
final geoStatesProvider = FutureProvider<List<GeoState>>(
  (ref) => ref.watch(locationIdentityApiProvider).states(),
);

/// Active districts of one state.
final geoDistrictsProvider = FutureProvider.family<List<District>, int>(
  (ref, stateId) => ref.watch(locationIdentityApiProvider).districts(stateId),
);

/// The caller's two identity slots, notice and allowed proof types.
final locationIdentitiesProvider = FutureProvider.autoDispose<LocationIdentities>(
  (ref) => ref.watch(locationIdentityApiProvider).mine(),
);

/// How a failed submit should be presented.
sealed class SubmitProblem {
  const SubmitProblem();

  static SubmitProblem? from(Object error) {
    if (error is! ApiException) return null;
    if (error.code == ApiErrorCode.limitReached) {
      return CooldownProblem(DateTime.tryParse(error.fields['nextChangeAt'] ?? ''));
    }
    if (error.code == ApiErrorCode.conflict) return const PendingExistsProblem();
    return null;
  }
}

/// 409 `LIMIT_REACHED`: a different residence district is in cooldown.
class CooldownProblem extends SubmitProblem {
  const CooldownProblem(this.nextChangeAt);
  final DateTime? nextChangeAt;
}

/// 409 `CONFLICT`: one is already in review; withdraw it first.
class PendingExistsProblem extends SubmitProblem {
  const PendingExistsProblem();
}

class LocationIdentityController {
  LocationIdentityController(this._ref);

  final Ref _ref;

  LocationIdentityApi get _api => _ref.read(locationIdentityApiProvider);

  /// Uploads the proof (purpose `location_proof`) and submits. The file
  /// bytes live only in memory for the upload.
  Future<LocationIdentity> submit({
    required String identityType,
    required int districtId,
    required String proofType,
    required UploadFile proof,
    required bool guardianProof,
    required int noticePolicyDocumentId,
  }) async {
    final uploadId = await _ref.read(mediaUploaderProvider).upload(MediaPurpose.locationProof, proof);
    final created = await _api.submit(LocationIdentityInput(
      identityType: identityType,
      districtId: districtId,
      proofType: proofType,
      proofUploadId: uploadId,
      proofHolder: guardianProof ? 'guardian' : 'self',
      noticePolicyDocumentId: noticePolicyDocumentId,
    ));
    _ref.invalidate(locationIdentitiesProvider);
    return created;
  }

  Future<void> withdraw(int id) async {
    await _api.withdraw(id);
    _ref.invalidate(locationIdentitiesProvider);
  }
}

final locationIdentityControllerProvider =
    Provider<LocationIdentityController>(LocationIdentityController.new);

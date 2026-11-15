import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/session/session_controller.dart';
import '../data/consultation_api.dart';
import '../data/consultation_models.dart';

final consultationOfferProvider = FutureProvider.autoDispose.family<ConsultationOffer, int>(
  (ref, coachId) => ref.watch(consultationApiProvider).offer(coachId),
);

/// My own offer as a coach (`GET /me/consultation`).
final myConsultationOfferProvider = FutureProvider.autoDispose<ConsultationOffer?>((ref) async {
  final id = ref.watch(currentUserIdProvider);
  if (id == null) return null;
  return ref.watch(consultationApiProvider).myOffer();
});

/// Writes for consultations. Each returns the updated request so lists can
/// patch the row in place.
class ConsultationController {
  ConsultationController(this._ref);

  final Ref _ref;

  ConsultationApi get _api => _ref.read(consultationApiProvider);

  Future<ConsultationOffer> saveOffer(ConsultationOfferInput input) async {
    final offer = await _api.updateMyOffer(input);
    _ref.invalidate(myConsultationOfferProvider);
    final me = _ref.read(currentUserIdProvider);
    if (me != null) _ref.invalidate(consultationOfferProvider(me));
    return offer;
  }

  Future<ConsultationRequest> request(int coachId, String message, String mode) async {
    final r = await _api.request(coachId, message: message, preferredMode: mode);
    _ref.invalidate(consultationOfferProvider(coachId));
    return r;
  }

  Future<ConsultationRequest> respond(int id, {required bool accept, String? note}) =>
      _api.respond(id, accept: accept, note: note);

  Future<ConsultationRequest> cancel(int id) => _api.cancel(id);

  Future<ConsultationRequest> complete(int id) => _api.complete(id);
}

final consultationControllerProvider =
    Provider<ConsultationController>(ConsultationController.new);
